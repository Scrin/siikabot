package aigateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"strconv"
	"time"

	"github.com/Scrin/siikabot/config"
	"github.com/Scrin/siikabot/metrics"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// callTimeout bounds a single attempt. A chat bot that has not heard back within this is not going
// to produce something the user still wants, and leaving the call open only delays the retry.
//
// Chosen so that a fully retried call still fits inside the caller's turn budget: three attempts
// plus backoff has to leave room for the turn to do something with the answer. TestRetryBudget
// FitsInsideTurn checks that relationship holds.
const callTimeout = 45 * time.Second

// httpTimeout is a backstop slightly above callTimeout, for the case where the per-attempt context
// somehow does not fire
const httpTimeout = 60 * time.Second

// gatewayTimeoutMS asks the gateway to give up at the same point the client does, so a stalled
// upstream is abandoned on both sides rather than only locally
const gatewayTimeoutMS = int(callTimeout / time.Millisecond)

// Retry settings for transient failures. Kept small deliberately: a user is waiting, and a request
// that has already failed twice is unlikely to succeed on a third attempt soon enough to matter.
const maxAttempts = 3
const retryBaseDelay = 500 * time.Millisecond
const retryMaxDelay = 4 * time.Second

// Gateway-side retry settings, applied by AI Gateway before the response ever reaches us. These
// cover a transient upstream blip without costing a client round trip.
const gatewayMaxAttempts = "2"
const gatewayRetryDelayMS = "500"
const gatewayBackoff = "exponential"

// All requests go through the /ai/run endpoint. It accepts the OpenAI chat completions
// body nested under "input" and returns the response wrapped in "result". The
// OpenAI-compatible endpoint (/ai/v1/chat/completions) is deliberately not used: it
// forwards image content parts to OpenAI models in a shape they reject.
const runPath = "/ai/run"

// Instrumented so each attempt produces a client span. Cloudflare's own span cannot be nested under
// it — the REST API discards the trace context we send — but the local timing is still worth having.
var httpClient = &http.Client{
	Timeout:   httpTimeout,
	Transport: otelhttp.NewTransport(http.DefaultTransport),
}

// ImageURL is the image payload of a content part.
//
// Detail selects the fidelity the provider renders the image at, and is the difference between a
// flat handful of tokens and several thousand for the same picture. Omitted when empty, which
// leaves the choice to the provider.
type ImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}

// ContentPart represents a part of a message content in the chat API
type ContentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
}

// Message represents a message in the chat API
type Message struct {
	Role       string     `json:"role"`
	Content    any        `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Refusal    any        `json:"refusal,omitempty"`
}

// ChatRequest represents a request to the chat API
type ChatRequest struct {
	Model     string           `json:"model"`
	Messages  []Message        `json:"messages"`
	Tools     []ToolDefinition `json:"tools,omitempty"`
	MaxTokens *int             `json:"max_tokens,omitempty"`

	// Metadata travels in the cf-aig-metadata header rather than the body, and Cloudflare turns it
	// into attributes on its own span and filterable fields in its logs
	Metadata map[string]string `json:"-"`
}

// chatInput is the "input" object of an /ai/run request, holding everything but the model
type chatInput struct {
	Messages  []Message        `json:"messages"`
	Tools     []ToolDefinition `json:"tools,omitempty"`
	MaxTokens *int             `json:"max_tokens,omitempty"`
}

// runRequest is the envelope the /ai/run endpoint expects
type runRequest struct {
	Model string    `json:"model"`
	Input chatInput `json:"input"`
}

// Choice represents a choice in the chat API response
type Choice struct {
	Message            Message `json:"message"`
	FinishReason       string  `json:"finish_reason,omitempty"`
	NativeFinishReason string  `json:"native_finish_reason,omitempty"`
	Index              int     `json:"index,omitempty"`
	LogProbs           any     `json:"logprobs"`
}

// PromptTokensDetails breaks down the prompt tokens of a response.
//
// CachedTokens is how much of the prompt the provider served from its cache. It is the only direct
// evidence that the stable-prefix work is doing anything, so it is worth reading even though nothing
// else in the response needs it.
type PromptTokensDetails struct {
	CachedTokens int `json:"cached_tokens"`
}

// Usage represents token usage information in the chat API response
type Usage struct {
	PromptTokens        int                  `json:"prompt_tokens"`
	CompletionTokens    int                  `json:"completion_tokens"`
	TotalTokens         int                  `json:"total_tokens"`
	PromptTokensDetails *PromptTokensDetails `json:"prompt_tokens_details,omitempty"`
}

// CachedPromptTokens returns how many prompt tokens were served from the provider's cache
func (u *Usage) CachedPromptTokens() int {
	if u == nil || u.PromptTokensDetails == nil {
		return 0
	}
	return u.PromptTokensDetails.CachedTokens
}

// ChatResponse represents a response from the chat API
type ChatResponse struct {
	ID                string   `json:"id,omitempty"`
	Model             string   `json:"model,omitempty"`
	Object            string   `json:"object,omitempty"`
	Created           int64    `json:"created,omitempty"`
	Choices           []Choice `json:"choices"`
	SystemFingerprint string   `json:"system_fingerprint,omitempty"`
	Usage             *Usage   `json:"usage,omitempty"`
}

// apiError represents a single error in a Cloudflare API response envelope
type apiError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// runResponse is the envelope the /ai/run endpoint returns, for both success and failure
type runResponse struct {
	Success bool         `json:"success"`
	Errors  []apiError   `json:"errors"`
	Result  ChatResponse `json:"result"`
}

// endpoint builds a full Cloudflare API URL for the configured account
func endpoint(path string) string {
	return fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s%s", config.CloudflareAccountID, path)
}

// setAuthHeaders sets the authentication and gateway headers required by the AI Gateway REST API.
// Note that unlike the legacy gateway.ai.cloudflare.com endpoints, the REST API takes the
// Cloudflare token in Authorization, not in cf-aig-authorization.
func setAuthHeaders(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+config.CloudflareAPIToken)
	req.Header.Set("cf-aig-gateway-id", config.CloudflareAIGatewayID)
}

// setInferenceHeaders sets the per-request gateway behaviour for an inference call: a timeout
// matching the client's own, and a small number of gateway-side retries that absorb a transient
// upstream failure without a client round trip
func setInferenceHeaders(req *http.Request) {
	req.Header.Set("cf-aig-request-timeout", strconv.Itoa(gatewayTimeoutMS))
	req.Header.Set("cf-aig-max-attempts", gatewayMaxAttempts)
	req.Header.Set("cf-aig-retry-delay", gatewayRetryDelayMS)
	req.Header.Set("cf-aig-backoff", gatewayBackoff)
}

// setMetadataHeader attaches request metadata for Cloudflare to record.
//
// This carries the trace correlation as well as the business context. Cloudflare's spans cannot be
// nested under ours — the REST API discards cf-aig-otel-trace-id — but metadata does survive, so the
// bot's trace and span ids ride along as attributes and the two traces can be matched up afterwards.
func setMetadataHeader(ctx context.Context, req *http.Request, metadata map[string]string) {
	combined := make(map[string]string, len(metadata)+2)
	for k, v := range metadata {
		combined[k] = v
	}

	// Guarded: an invalid span context would otherwise write all-zero ids that match no trace
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		combined["siikabot_trace_id"] = sc.TraceID().String()
		combined["siikabot_span_id"] = sc.SpanID().String()
	}

	if len(combined) == 0 {
		return
	}

	encoded, err := json.Marshal(combined)
	if err != nil {
		log.Warn().Ctx(ctx).Err(err).Msg("Failed to encode gateway metadata")
		return
	}
	req.Header.Set("cf-aig-metadata", string(encoded))
}

// firstError returns the code and message of the first error in an API response envelope
func firstError(errs []apiError) (int, string) {
	if len(errs) == 0 {
		return 0, ""
	}
	return errs[0].Code, errs[0].Message
}

// SendChatRequest sends a request to the AI Gateway chat API
func SendChatRequest(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	jsonData, err := json.Marshal(runRequest{
		Model: req.Model,
		Input: chatInput{
			Messages:  req.Messages,
			Tools:     req.Tools,
			MaxTokens: req.MaxTokens,
		},
	})
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("model", req.Model).Msg("Failed to marshal chat request")
		return nil, fmt.Errorf("failed to marshal chat request: %w", err)
	}

	ctx, span := tracer.Start(ctx, "gen_ai.chat "+req.Model,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String(attrOperationName, operationChat),
			attribute.String(attrProviderName, providerFromModel(req.Model)),
			attribute.String(attrRequestModel, req.Model),
		))
	defer span.End()

	// Retry transient failures. Only the HTTP call is retried, never anything around it: by the
	// time a caller is in a tool loop the tools have already run, and some of them write.
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		chatResp, kind, err := sendOnce(ctx, req.Model, req.Metadata, jsonData, attempt)
		if err == nil {
			recordResponseAttributes(span, chatResp)
			return chatResp, nil
		}
		lastErr = err

		if !isRetryable(kind) || attempt == maxAttempts {
			break
		}
		if !waitBeforeRetry(ctx, attempt) {
			// The turn's deadline passed or it was cancelled, so there is nobody left to answer
			break
		}

		metrics.RecordChatAPIRetry(req.Model, string(kind))
		span.AddEvent("retry", trace.WithAttributes(
			attribute.Int(attrAttempt, attempt),
			attribute.String("error_kind", string(kind)),
		))
		log.Warn().Ctx(ctx).
			Str("model", req.Model).
			Str("error_kind", string(kind)).
			Int("attempt", attempt).
			Int("max_attempts", maxAttempts).
			Msg("Retrying chat request after a transient failure")
	}

	span.RecordError(lastErr)
	span.SetStatus(codes.Error, lastErr.Error())
	return nil, lastErr
}

// recordResponseAttributes copies the parts of a successful response worth having on the span
func recordResponseAttributes(span trace.Span, resp *ChatResponse) {
	if resp == nil {
		return
	}
	if resp.Model != "" {
		span.SetAttributes(attribute.String(attrResponseModel, resp.Model))
	}
	if len(resp.Choices) > 0 && resp.Choices[0].FinishReason != "" {
		span.SetAttributes(attribute.StringSlice(attrFinishReason, []string{resp.Choices[0].FinishReason}))
	}
	if resp.Usage != nil {
		span.SetAttributes(
			attribute.Int(attrInputTokens, resp.Usage.PromptTokens),
			attribute.Int(attrOutputTokens, resp.Usage.CompletionTokens),
			attribute.Int(attrCachedInputTokens, resp.Usage.CachedPromptTokens()),
		)
	}
}

// isRetryable reports whether a failure is worth another attempt. Anything caused by the request
// itself will fail identically the second time, so only transient conditions qualify.
func isRetryable(kind ErrorKind) bool {
	switch kind {
	case ErrorKindNetwork, ErrorKindTimeout, ErrorKindRateLimited, ErrorKindProviderError:
		return true
	default:
		return false
	}
}

// retryDelay returns the backoff for an attempt: exponential growth up to a ceiling, jittered
// between half and one and a half of the nominal delay so concurrent turns do not retry in lockstep
func retryDelay(attempt int) time.Duration {
	delay := min(retryBaseDelay<<(attempt-1), retryMaxDelay)
	return time.Duration(float64(delay) * (0.5 + rand.Float64()))
}

// waitBeforeRetry sleeps for the backoff delay, returning false if the context finished first
func waitBeforeRetry(ctx context.Context, attempt int) bool {
	timer := time.NewTimer(retryDelay(attempt))
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// sendOnce performs a single attempt, returning the response or the failure with its classification
func sendOnce(ctx context.Context, model string, metadata map[string]string, jsonData []byte, attempt int) (*ChatResponse, ErrorKind, error) {
	ctx, span := tracer.Start(ctx, "gen_ai.chat.attempt",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attribute.Int(attrAttempt, attempt)))
	defer span.End()

	// Each attempt gets its own timeout, bounded by the caller's deadline for the whole turn
	callCtx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(callCtx, "POST", endpoint(runPath), bytes.NewBuffer(jsonData))
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("model", model).Msg("Failed to create HTTP request")
		return nil, ErrorKindUnknown, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	setAuthHeaders(httpReq)
	setInferenceHeaders(httpReq)
	// Uses the attempt's context, so the correlation ids point at this attempt's span
	setMetadataHeader(ctx, httpReq, metadata)

	// Measured around the call itself, so a slow turn can be attributed to the model rather than to
	// tool execution without reading logs
	startTime := time.Now()
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		kind := classifyTransportError(err)
		recordFailure(ctx, model, kind, startTime)
		log.Error().Ctx(ctx).Err(err).Str("model", model).Msg("Failed to send chat request")
		return nil, kind, fmt.Errorf("failed to send chat request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		kind := classifyTransportError(err)
		recordFailure(ctx, model, kind, startTime)
		log.Error().Ctx(ctx).Err(err).Str("model", model).Msg("Failed to read chat response")
		return nil, kind, fmt.Errorf("failed to read chat response: %w", err)
	}

	var runResp runResponse
	if err := json.Unmarshal(body, &runResp); err != nil {
		recordFailure(ctx, model, ErrorKindParseError, startTime)
		log.Error().Ctx(ctx).Err(err).
			Str("model", model).
			Int("status_code", resp.StatusCode).
			Str("response", string(body)).
			Msg("Failed to parse chat response")
		return nil, ErrorKindParseError, fmt.Errorf("failed to parse chat response: %w", err)
	}

	// Failures are reported both by the HTTP status and by the envelope's success field
	if resp.StatusCode >= 400 || !runResp.Success || len(runResp.Errors) > 0 {
		errorCode, errorMessage := firstError(runResp.Errors)
		errorKind := classifyResponseError(resp.StatusCode, runResp.Errors)
		recordFailure(ctx, model, errorKind, startTime)
		log.Error().Ctx(ctx).
			Str("model", model).
			Int("status_code", resp.StatusCode).
			Int("error_code", errorCode).
			Str("error_kind", string(errorKind)).
			Str("error_message", errorMessage).
			Str("response", string(body)).
			Msg("Chat API returned error")
		if errorMessage == "" {
			return nil, errorKind, fmt.Errorf("chat API error: HTTP %d", resp.StatusCode)
		}
		return nil, errorKind, fmt.Errorf("chat API error: %s", errorMessage)
	}

	chatResp := runResp.Result

	log.Trace().Ctx(ctx).
		Str("model", model).
		Str("response", string(body)).
		Msg("Chat API response")

	metrics.RecordChatAPICall(model, true)
	metrics.RecordChatAPICallDuration(model, time.Since(startTime).Seconds())

	if chatResp.Usage != nil {
		cachedTokens := chatResp.Usage.CachedPromptTokens()
		log.Debug().Ctx(ctx).
			Str("model", model).
			Int("prompt_tokens", chatResp.Usage.PromptTokens).
			Int("completion_tokens", chatResp.Usage.CompletionTokens).
			Int("cached_prompt_tokens", cachedTokens).
			Int("total_tokens", chatResp.Usage.TotalTokens).
			Msg("Chat API token usage")
		metrics.RecordChatTokens(model, chatResp.Usage.PromptTokens, chatResp.Usage.CompletionTokens)
		metrics.RecordChatCachedTokens(model, cachedTokens, chatResp.Usage.PromptTokens)
	}

	return &chatResp, "", nil
}

// recordFailure records the metrics for a failed chat request
func recordFailure(ctx context.Context, model string, kind ErrorKind, startTime time.Time) {
	metrics.RecordChatAPICall(model, false)
	metrics.RecordChatAPIError(model, string(kind))
	metrics.RecordChatAPICallDuration(model, time.Since(startTime).Seconds())
	log.Debug().Ctx(ctx).
		Str("model", model).
		Str("error_kind", string(kind)).
		Msg("Chat request failed")
}

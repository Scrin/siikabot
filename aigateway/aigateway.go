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
	"strings"
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

// Instrumented so each attempt produces a client span, which Cloudflare's own span then nests
// under by way of the trace headers set on the request
var httpClient = &http.Client{
	Timeout:   httpTimeout,
	Transport: otelhttp.NewTransport(http.DefaultTransport),
}

// chatCompletionsURL builds the Unified API URL for the configured account and gateway.
//
// This is the OpenAI-compatible endpoint on gateway.ai.cloudflare.com rather than the newer REST
// API on api.cloudflare.com, and the choice is deliberate: it is the only endpoint that honours
// cf-aig-otel-trace-id, which is what lets Cloudflare's span join this trace instead of sitting in
// one of its own. Verified by sending a known trace id to all three candidate endpoints — see T10
// in OTEL_TRACING_PLAN.md, which also records the probe showing tool calling, image input and
// cached-token accounting all work here.
//
// Cloudflare marks this endpoint deprecated for single-model chat completions while keeping it
// required for dynamic routing, so it is not going away soon. Should it ever be withdrawn, the way
// back is /ai/run with the body nested under "input" and the response unwrapped from "result",
// giving up span nesting and falling back to the metadata correlation that is still sent below.
//
// Note that /ai/v1/chat/completions is not an alternative even setting tracing aside: it forwards
// image content parts to OpenAI models in a shape they reject.
func chatCompletionsURL() string {
	return fmt.Sprintf("https://gateway.ai.cloudflare.com/v1/%s/%s/compat/chat/completions",
		config.CloudflareAccountID, config.CloudflareAIGatewayID)
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

// providerError is the error object a provider returns through the compat endpoint.
//
// Its code is a string where Cloudflare's is numeric, so it is kept as an any for logging rather
// than forced into apiError. Classification does not need it: that keys on the HTTP status and the
// message, both of which are present either way.
type providerError struct {
	Message string `json:"message"`
	Type    string `json:"type,omitempty"`
	Code    any    `json:"code,omitempty"`
}

// chatCompletionResponse is what the compat endpoint returns: OpenAI's chat completion object at
// the top level, with no Cloudflare envelope wrapped around it.
//
// Two error shapes have to be accepted because a request can fail on either side of the gateway.
// The provider's own failures arrive as OpenAI's "error" object, while the gateway's arrive in
// Cloudflare's "errors" array, and neither is reported by the other.
type chatCompletionResponse struct {
	ChatResponse
	Error  *providerError `json:"error,omitempty"`
	Errors []apiError     `json:"errors,omitempty"`
}

// failed reports whether the response carries an error in either shape
func (r chatCompletionResponse) failed() bool {
	return r.Error != nil || len(r.Errors) > 0
}

// apiErrors normalises whichever error shape arrived into the form classifyResponseError expects
func (r chatCompletionResponse) apiErrors() []apiError {
	if r.Error != nil {
		return []apiError{{Message: r.Error.Message}}
	}
	return r.Errors
}

// errorCode returns the provider's string code, or the gateway's numeric one rendered as a string.
// Only used for logging, where knowing which of the two arrived is itself informative.
func (r chatCompletionResponse) errorCode() string {
	if r.Error != nil && r.Error.Code != nil {
		return fmt.Sprint(r.Error.Code)
	}
	if code, _ := firstError(r.Errors); code != 0 {
		return strconv.Itoa(code)
	}
	return ""
}

// endpoint builds a full Cloudflare REST API URL for the configured account.
//
// Used for the management API — the log poller — and not for inference, which goes to the Unified
// API instead. See chatCompletionsURL.
func endpoint(path string) string {
	return fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s%s", config.CloudflareAccountID, path)
}

// setManagementAuthHeader authenticates a call to the REST management API on api.cloudflare.com,
// which takes the Cloudflare token in Authorization
func setManagementAuthHeader(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+config.CloudflareAPIToken)
}

// setInferenceAuthHeader authenticates a call to the Unified API, which takes the Cloudflare token
// in cf-aig-authorization instead.
//
// Authorization is deliberately left unset: that header is where a provider's own key would go, and
// Unified Billing means we do not hold one. The gateway is named in the URL here rather than in a
// cf-aig-gateway-id header.
func setInferenceAuthHeader(req *http.Request) {
	req.Header.Set("cf-aig-authorization", "Bearer "+config.CloudflareAPIToken)
}

// setTraceHeaders asks Cloudflare to emit its span as a child of the current one.
//
// This is what puts the gateway's own view of a call — provider-side timing, gen_ai.usage.cost, the
// full prompt and completion — into the same waterfall as the rest of the turn, rather than in a
// separate trace reachable only by correlation.
//
// Guarded like the metadata below: without a valid span context these would be all-zero ids, and
// Cloudflare would dutifully attach its span to a trace that does not exist.
func setTraceHeaders(ctx context.Context, req *http.Request) {
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		return
	}
	req.Header.Set("cf-aig-otel-trace-id", sc.TraceID().String())
	req.Header.Set("cf-aig-otel-parent-span-id", sc.SpanID().String())
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
// The business context is the point of it. The trace and span ids are also included, which is now
// redundant with the trace headers doing the real nesting, but kept for two reasons: metadata is
// the only one of the two that also reaches the gateway logs, where it makes a log entry traceable
// back to a turn; and it keeps the association working on its own should the trace headers ever
// stop being honoured, which is exactly how the REST API behaves.
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

// maxBodyMessageLength caps how much of a non-JSON error body is repeated into an error message.
// Provider errors are usually a short sentence, but nothing guarantees it is not an HTML page.
const maxBodyMessageLength = 200

// errorMessageFromBody turns a response body that is not JSON into something worth reporting.
//
// The body is the only description available in that case, and it is often the most useful one:
// "Authentication Fails (governor)" says precisely what went wrong, where a generic "HTTP 401" would
// leave the reader guessing which side rejected the request.
func errorMessageFromBody(body []byte) string {
	message := strings.TrimSpace(string(body))
	if message == "" {
		return ""
	}

	// Truncated by runes rather than bytes, so a multi-byte character is never cut in half
	if runes := []rune(message); len(runes) > maxBodyMessageLength {
		return string(runes[:maxBodyMessageLength]) + "..."
	}
	return message
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
	// ChatRequest is already the OpenAI request shape the compat endpoint expects, and Metadata is
	// tagged json:"-" because it travels as a header, so this marshals to exactly the right body
	jsonData, err := json.Marshal(req)
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

	httpReq, err := http.NewRequestWithContext(callCtx, "POST", chatCompletionsURL(), bytes.NewBuffer(jsonData))
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("model", model).Msg("Failed to create HTTP request")
		return nil, ErrorKindUnknown, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	setInferenceAuthHeader(httpReq)
	setInferenceHeaders(httpReq)
	// Both use the attempt's context, so Cloudflare's span hangs off this attempt rather than off
	// the enclosing call — a retry is then visibly its own subtree
	setTraceHeaders(ctx, httpReq)
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

	var compatResp chatCompletionResponse
	parseErr := json.Unmarshal(body, &compatResp)

	// A failure shows up either in the HTTP status or in an error object in the body. There is no
	// success flag on this endpoint, so an error-shaped body is the only signal when the status
	// itself is 200 — which happens when the gateway succeeds but the provider behind it does not.
	//
	// The status is checked before the body is required to parse, because a failed response is not
	// guaranteed to be JSON. The compat endpoint forwards some provider errors through verbatim, and
	// not every provider answers in JSON: DeepSeek rejects a bad credential with the bare string
	// "Authentication Fails (governor)". Parsing first turned that plain 401 into a parse_error,
	// which reported the wrong metric and buried the actual cause. The /ai/run endpoint always
	// wrapped errors in Cloudflare's own envelope, so this only became reachable on this endpoint.
	if resp.StatusCode >= 400 || (parseErr == nil && compatResp.failed()) {
		errs := compatResp.apiErrors()
		if parseErr != nil {
			// Nothing structured to read, so the body itself is the best description available
			errs = []apiError{{Message: errorMessageFromBody(body)}}
		}
		_, errorMessage := firstError(errs)
		errorKind := classifyResponseError(resp.StatusCode, errs)
		recordFailure(ctx, model, errorKind, startTime)
		log.Error().Ctx(ctx).
			Str("model", model).
			Int("status_code", resp.StatusCode).
			Str("error_code", compatResp.errorCode()).
			Str("error_kind", string(errorKind)).
			Str("error_message", errorMessage).
			Str("response", string(body)).
			Msg("Chat API returned error")
		if errorMessage == "" {
			return nil, errorKind, fmt.Errorf("chat API error: HTTP %d", resp.StatusCode)
		}
		return nil, errorKind, fmt.Errorf("chat API error: %s", errorMessage)
	}

	// A successful status with a body that will not parse is a genuine parse error
	if parseErr != nil {
		recordFailure(ctx, model, ErrorKindParseError, startTime)
		log.Error().Ctx(ctx).Err(parseErr).
			Str("model", model).
			Int("status_code", resp.StatusCode).
			Str("response", string(body)).
			Msg("Failed to parse chat response")
		return nil, ErrorKindParseError, fmt.Errorf("failed to parse chat response: %w", parseErr)
	}

	chatResp := compatResp.ChatResponse

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

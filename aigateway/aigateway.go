package aigateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Scrin/siikabot/config"
	"github.com/Scrin/siikabot/metrics"
	"github.com/rs/zerolog/log"
)

// requestTimeout is the maximum time to wait for a single inference request
const requestTimeout = 5 * time.Minute

// All requests go through the /ai/run endpoint. It accepts the OpenAI chat completions
// body nested under "input" and returns the response wrapped in "result". The
// OpenAI-compatible endpoint (/ai/v1/chat/completions) is deliberately not used: it
// forwards image content parts to OpenAI models in a shape they reject.
const runPath = "/ai/run"

var httpClient = &http.Client{Timeout: requestTimeout}

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

	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint(runPath), bytes.NewBuffer(jsonData))
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("model", req.Model).Msg("Failed to create HTTP request")
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	setAuthHeaders(httpReq)

	// Measured around the call itself, so a slow turn can be attributed to the model rather than to
	// tool execution without reading logs
	startTime := time.Now()
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		recordFailure(ctx, req.Model, classifyTransportError(err), startTime)
		log.Error().Ctx(ctx).Err(err).Str("model", req.Model).Msg("Failed to send chat request")
		return nil, fmt.Errorf("failed to send chat request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		recordFailure(ctx, req.Model, classifyTransportError(err), startTime)
		log.Error().Ctx(ctx).Err(err).Str("model", req.Model).Msg("Failed to read chat response")
		return nil, fmt.Errorf("failed to read chat response: %w", err)
	}

	var runResp runResponse
	if err := json.Unmarshal(body, &runResp); err != nil {
		recordFailure(ctx, req.Model, ErrorKindParseError, startTime)
		log.Error().Ctx(ctx).Err(err).
			Str("model", req.Model).
			Int("status_code", resp.StatusCode).
			Str("response", string(body)).
			Msg("Failed to parse chat response")
		return nil, fmt.Errorf("failed to parse chat response: %w", err)
	}

	// Failures are reported both by the HTTP status and by the envelope's success field
	if resp.StatusCode >= 400 || !runResp.Success || len(runResp.Errors) > 0 {
		errorCode, errorMessage := firstError(runResp.Errors)
		errorKind := classifyResponseError(resp.StatusCode, runResp.Errors)
		recordFailure(ctx, req.Model, errorKind, startTime)
		log.Error().Ctx(ctx).
			Str("model", req.Model).
			Int("status_code", resp.StatusCode).
			Int("error_code", errorCode).
			Str("error_kind", string(errorKind)).
			Str("error_message", errorMessage).
			Str("response", string(body)).
			Msg("Chat API returned error")
		if errorMessage == "" {
			return nil, fmt.Errorf("chat API error: HTTP %d", resp.StatusCode)
		}
		return nil, fmt.Errorf("chat API error: %s", errorMessage)
	}

	chatResp := runResp.Result

	log.Trace().Ctx(ctx).
		Str("model", req.Model).
		Str("response", string(body)).
		Msg("Chat API response")

	metrics.RecordChatAPICall(req.Model, true)
	metrics.RecordChatAPICallDuration(req.Model, time.Since(startTime).Seconds())

	if chatResp.Usage != nil {
		cachedTokens := chatResp.Usage.CachedPromptTokens()
		log.Debug().Ctx(ctx).
			Str("model", req.Model).
			Int("prompt_tokens", chatResp.Usage.PromptTokens).
			Int("completion_tokens", chatResp.Usage.CompletionTokens).
			Int("cached_prompt_tokens", cachedTokens).
			Int("total_tokens", chatResp.Usage.TotalTokens).
			Msg("Chat API token usage")
		metrics.RecordChatTokens(req.Model, chatResp.Usage.PromptTokens, chatResp.Usage.CompletionTokens)
		metrics.RecordChatCachedTokens(req.Model, cachedTokens, chatResp.Usage.PromptTokens)
	}

	return &chatResp, nil
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

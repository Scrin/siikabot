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

// ContentPart represents a part of a message content in the chat API
type ContentPart struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL *struct {
		URL string `json:"url"`
	} `json:"image_url,omitempty"`
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

// Usage represents token usage information in the chat API response
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
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

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("model", req.Model).Msg("Failed to send chat request")
		metrics.RecordChatAPICall(req.Model, false)
		return nil, fmt.Errorf("failed to send chat request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("model", req.Model).Msg("Failed to read chat response")
		metrics.RecordChatAPICall(req.Model, false)
		return nil, fmt.Errorf("failed to read chat response: %w", err)
	}

	var runResp runResponse
	if err := json.Unmarshal(body, &runResp); err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("model", req.Model).
			Int("status_code", resp.StatusCode).
			Str("response", string(body)).
			Msg("Failed to parse chat response")
		metrics.RecordChatAPICall(req.Model, false)
		return nil, fmt.Errorf("failed to parse chat response: %w", err)
	}

	// Failures are reported both by the HTTP status and by the envelope's success field
	if resp.StatusCode >= 400 || !runResp.Success || len(runResp.Errors) > 0 {
		errorCode, errorMessage := firstError(runResp.Errors)
		log.Error().Ctx(ctx).
			Str("model", req.Model).
			Int("status_code", resp.StatusCode).
			Int("error_code", errorCode).
			Str("error_message", errorMessage).
			Str("response", string(body)).
			Msg("Chat API returned error")
		metrics.RecordChatAPICall(req.Model, false)
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

	if chatResp.Usage != nil {
		log.Debug().Ctx(ctx).
			Str("model", req.Model).
			Int("prompt_tokens", chatResp.Usage.PromptTokens).
			Int("completion_tokens", chatResp.Usage.CompletionTokens).
			Int("total_tokens", chatResp.Usage.TotalTokens).
			Msg("Chat API token usage")
		metrics.RecordChatTokens(req.Model, chatResp.Usage.PromptTokens, chatResp.Usage.CompletionTokens)
	}

	return &chatResp, nil
}

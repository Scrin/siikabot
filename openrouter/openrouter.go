package openrouter

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

// ContentPart represents a part of a message content in the OpenRouter chat API
type ContentPart struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL *struct {
		URL string `json:"url"`
	} `json:"image_url,omitempty"`
}

// Message represents a message in the OpenRouter chat API
type Message struct {
	Role       string      `json:"role"`
	Content    interface{} `json:"content"`
	ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`
	ToolCallID string      `json:"tool_call_id,omitempty"`
	Refusal    interface{} `json:"refusal,omitempty"`
}

// ChatRequest represents a request to the OpenRouter chat API
type ChatRequest struct {
	Model    string           `json:"model"`
	Messages []Message        `json:"messages"`
	Tools    []ToolDefinition `json:"tools,omitempty"`
}

// Choice represents a choice in the OpenRouter chat API response
type Choice struct {
	Message            Message     `json:"message"`
	FinishReason       string      `json:"finish_reason,omitempty"`
	NativeFinishReason string      `json:"native_finish_reason,omitempty"`
	Index              int         `json:"index,omitempty"`
	LogProbs           interface{} `json:"logprobs"`
}

// Usage represents token usage information in the OpenRouter chat API response
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ChatResponse represents a response from the OpenRouter chat API
type ChatResponse struct {
	ID                string   `json:"id,omitempty"`
	Provider          string   `json:"provider,omitempty"`
	Model             string   `json:"model,omitempty"`
	Object            string   `json:"object,omitempty"`
	Created           int64    `json:"created,omitempty"`
	Choices           []Choice `json:"choices"`
	SystemFingerprint string   `json:"system_fingerprint,omitempty"`
	Usage             *Usage   `json:"usage,omitempty"`
	Error             *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

// SendChatRequest sends a request to the OpenRouter chat API
func SendChatRequest(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	jsonData, err := json.Marshal(req)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to marshal chat request")
		return nil, fmt.Errorf("failed to marshal chat request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", "https://openrouter.ai/api/v1/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to create HTTP request")
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+config.OpenrouterAPIKey)
	httpReq.Header.Set("HTTP-Referer", "https://github.com/Scrin/siikabot")
	httpReq.Header.Set("X-Title", "Siikabot")

	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to send chat request")
		metrics.RecordChatAPICall(req.Model, false)
		return nil, fmt.Errorf("failed to send chat request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to read chat response")
		metrics.RecordChatAPICall(req.Model, false)
		return nil, fmt.Errorf("failed to read chat response: %w", err)
	}

	var chatResp ChatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		log.Error().Ctx(ctx).Err(err).Str("response", string(body)).Msg("Failed to parse chat response")
		metrics.RecordChatAPICall(req.Model, false)
		return nil, fmt.Errorf("failed to parse chat response: %w", err)
	}

	if chatResp.Error != nil {
		log.Error().Ctx(ctx).
			Str("error_type", chatResp.Error.Type).
			Str("error_message", chatResp.Error.Message).
			Str("model", req.Model).
			Str("response", string(body)).
			Msg("Chat API returned error")
		metrics.RecordChatAPICall(req.Model, false)
		return &chatResp, fmt.Errorf("chat API error: %s", chatResp.Error.Message)
	}

	log.Trace().Ctx(ctx).
		Str("model", req.Model).
		Str("response", string(body)).
		Msg("Chat API response")

	// Record successful API call
	metrics.RecordChatAPICall(req.Model, true)

	// Record token usage if available
	if chatResp.Usage != nil {
		log.Debug().Ctx(ctx).
			Str("model", req.Model).
			Int("prompt_tokens", chatResp.Usage.PromptTokens).
			Int("completion_tokens", chatResp.Usage.CompletionTokens).
			Int("total_tokens", chatResp.Usage.TotalTokens).
			Msg("Chat API token usage")
		metrics.RecordChatTokens(req.Model, chatResp.Usage.PromptTokens, chatResp.Usage.CompletionTokens)
	}

	// Fetch generation stats in a background goroutine
	if chatResp.ID != "" {
		go fetchGenerationStats(context.Background(), chatResp.ID, req.Model, chatResp.Provider)
	}

	return &chatResp, nil
}

// GenerationStats represents the statistics from the OpenRouter generation API
type GenerationStats struct {
	Data struct {
		ID                     string      `json:"id"`
		UpstreamID             string      `json:"upstream_id"`
		TotalCost              float64     `json:"total_cost"`
		CacheDiscount          interface{} `json:"cache_discount"`
		ProviderName           string      `json:"provider_name"`
		CreatedAt              string      `json:"created_at"`
		Model                  string      `json:"model"`
		AppID                  int         `json:"app_id"`
		Streamed               bool        `json:"streamed"`
		Cancelled              bool        `json:"cancelled"`
		Latency                int         `json:"latency"`
		ModerationLatency      int         `json:"moderation_latency"`
		GenerationTime         int         `json:"generation_time"`
		TokensPrompt           int         `json:"tokens_prompt"`
		TokensCompletion       int         `json:"tokens_completion"`
		NativeTokensPrompt     int         `json:"native_tokens_prompt"`
		NativeTokensCompletion int         `json:"native_tokens_completion"`
		NativeTokensReasoning  int         `json:"native_tokens_reasoning"`
		NumMediaPrompt         interface{} `json:"num_media_prompt"`
		NumMediaCompletion     interface{} `json:"num_media_completion"`
		NumSearchResults       interface{} `json:"num_search_results"`
		Origin                 string      `json:"origin"`
		IsByok                 bool        `json:"is_byok"`
		FinishReason           string      `json:"finish_reason"`
		NativeFinishReason     string      `json:"native_finish_reason"`
		Usage                  float64     `json:"usage"`
	} `json:"data"`
}

// fetchGenerationStats fetches statistics from the OpenRouter generation API
func fetchGenerationStats(ctx context.Context, generationID, model, provider string) {
	// Wait a short time to ensure the generation stats are available
	time.Sleep(1 * time.Second)

	url := fmt.Sprintf("https://openrouter.ai/api/v1/generation?id=%s", generationID)

	httpReq, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("generation_id", generationID).Msg("Failed to create HTTP request for generation stats")
		return
	}

	httpReq.Header.Set("Authorization", "Bearer "+config.OpenrouterAPIKey)
	httpReq.Header.Set("HTTP-Referer", "https://github.com/Scrin/siikabot")
	httpReq.Header.Set("X-Title", "Siikabot")

	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("generation_id", generationID).Msg("Failed to fetch generation stats")
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("generation_id", generationID).Msg("Failed to read generation stats response")
		return
	}

	var stats GenerationStats
	if err := json.Unmarshal(body, &stats); err != nil {
		log.Error().Ctx(ctx).Err(err).Str("generation_id", generationID).Str("response", string(body)).Msg("Failed to parse generation stats")
		return
	}

	// Calculate latencies in seconds
	latencySec := float64(stats.Data.Latency) / 1000.0
	generationTimeSec := float64(stats.Data.GenerationTime) / 1000.0
	moderationLatencySec := float64(stats.Data.ModerationLatency) / 1000.0

	log.Debug().Ctx(ctx).
		Str("generation_id", generationID).
		Str("model", stats.Data.Model).
		Str("provider", stats.Data.ProviderName).
		Float64("latency_sec", latencySec).
		Float64("generation_time_sec", generationTimeSec).
		Float64("moderation_latency_sec", moderationLatencySec).
		Float64("total_cost", stats.Data.TotalCost).
		Int("tokens_prompt", stats.Data.TokensPrompt).
		Int("tokens_completion", stats.Data.TokensCompletion).
		Msg("Recorded OpenRouter generation stats")

	// Record the metrics
	metrics.RecordOpenRouterStats(stats.Data.Model, stats.Data.ProviderName, latencySec, generationTimeSec, moderationLatencySec, stats.Data.TotalCost)
	metrics.RecordOpenRouterTokens(stats.Data.Model, stats.Data.ProviderName, stats.Data.TokensPrompt, stats.Data.TokensCompletion)
}

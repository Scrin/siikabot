package openrouter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/Scrin/siikabot/config"
	"github.com/Scrin/siikabot/metrics"
	"github.com/rs/zerolog/log"
)

// Message represents a message in the OpenRouter chat API
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// ChatRequest represents a request to the OpenRouter chat API
type ChatRequest struct {
	Model    string           `json:"model"`
	Messages []Message        `json:"messages"`
	Tools    []ToolDefinition `json:"tools,omitempty"`
}

// Choice represents a choice in the OpenRouter chat API response
type Choice struct {
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason,omitempty"`
}

// ChatResponse represents a response from the OpenRouter chat API
type ChatResponse struct {
	Choices []Choice `json:"choices"`
	Error   *struct {
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
		log.Error().Ctx(ctx).Err(err).RawJSON("response", body).Msg("Failed to parse chat response")
		metrics.RecordChatAPICall(req.Model, false)
		return nil, fmt.Errorf("failed to parse chat response: %w", err)
	}

	if chatResp.Error != nil {
		log.Error().Ctx(ctx).
			Str("error_type", chatResp.Error.Type).
			Str("error_message", chatResp.Error.Message).
			Msg("Chat API returned error")
		metrics.RecordChatAPICall(req.Model, false)
		return &chatResp, fmt.Errorf("chat API error: %s", chatResp.Error.Message)
	}

	// Record successful API call
	metrics.RecordChatAPICall(req.Model, true)

	return &chatResp, nil
}

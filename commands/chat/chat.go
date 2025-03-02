package chat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Scrin/siikabot/config"
	"github.com/Scrin/siikabot/matrix"
	"github.com/rs/zerolog/log"
)

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model    string    `json:"model"`
	Messages []message `json:"messages"`
}

type choice struct {
	Message message `json:"message"`
}

type chatResponse struct {
	Choices []choice `json:"choices"`
	Error   *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

// Handle handles the chat command
func Handle(roomID, sender, msg string) {
	if strings.TrimSpace(msg) == "" {
		return
	}

	log.Debug().
		Str("room_id", roomID).
		Str("sender", sender).
		Msg("Processing chat command")

	req := chatRequest{
		Model: "google/gemini-2.0-flash-lite-preview-02-05:free",
		Messages: []message{
			{Role: "user", Content: msg},
		},
	}

	jsonData, err := json.Marshal(req)
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal chat request")
		matrix.SendMessage(roomID, "Failed to process chat request")
		return
	}

	httpReq, err := http.NewRequest("POST", "https://openrouter.ai/api/v1/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		log.Error().Err(err).Msg("Failed to create HTTP request")
		matrix.SendMessage(roomID, "Failed to create chat request")
		return
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+config.OpenrouterAPIKey)
	httpReq.Header.Set("HTTP-Referer", "https://github.com/Scrin/siikabot")
	httpReq.Header.Set("X-Title", "Siikabot")

	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		log.Error().Err(err).Msg("Failed to send chat request")
		matrix.SendMessage(roomID, "Failed to send chat request")
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error().Err(err).Msg("Failed to read chat response")
		matrix.SendMessage(roomID, "Failed to read chat response")
		return
	}

	var chatResp chatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		log.Error().Err(err).RawJSON("response", body).Msg("Failed to parse chat response")
		matrix.SendMessage(roomID, "Failed to parse chat response")
		return
	}

	if chatResp.Error != nil {
		log.Error().
			Str("error_type", chatResp.Error.Type).
			Str("error_message", chatResp.Error.Message).
			Msg("Chat API returned error")
		matrix.SendMessage(roomID, fmt.Sprintf("Chat API error: %s", chatResp.Error.Message))
		return
	}

	if len(chatResp.Choices) == 0 {
		log.Error().Msg("Chat API returned no choices")
		matrix.SendMessage(roomID, "No response from chat API")
		return
	}

	log.Debug().
		Str("room_id", roomID).
		Str("sender", sender).
		Int("response_length", len(chatResp.Choices[0].Message.Content)).
		Msg("Chat command completed")

	matrix.SendMessage(roomID, chatResp.Choices[0].Message.Content)
}

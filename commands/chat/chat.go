package chat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Scrin/siikabot/matrix"
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

var apiKey string

// Init initializes the chat command with the API key
func Init(openrouterAPIKey string) {
	apiKey = openrouterAPIKey
}

// Handle handles the chat command
func Handle(roomID, sender, msg string) {
	if strings.TrimSpace(msg) == "" {
		return
	}

	req := chatRequest{
		Model: "google/gemini-2.0-flash-lite-preview-02-05:free",
		Messages: []message{
			{
				Role:    "user",
				Content: msg,
			},
		},
	}

	jsonData, err := json.Marshal(req)
	if err != nil {
		matrix.SendMessage(roomID, fmt.Sprintf("Error marshaling request: %v", err))
		return
	}

	httpReq, err := http.NewRequest("POST", "https://openrouter.ai/api/v1/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		matrix.SendMessage(roomID, fmt.Sprintf("Error creating request: %v", err))
		return
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	httpClient := &http.Client{}
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		matrix.SendMessage(roomID, fmt.Sprintf("Error making request: %v", err))
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		matrix.SendMessage(roomID, fmt.Sprintf("Error reading response: %v", err))
		return
	}

	var chatResp chatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		matrix.SendMessage(roomID, fmt.Sprintf("Error parsing response: %v", err))
		return
	}

	if chatResp.Error != nil {
		matrix.SendMessage(roomID, fmt.Sprintf("API Error: %s (Type: %s)", chatResp.Error.Message, chatResp.Error.Type))
		return
	}

	if len(chatResp.Choices) > 0 {
		response := fmt.Sprintf("<a href=\"https://matrix.to/#/%s\">%s</a>: %s", sender, sender, chatResp.Choices[0].Message.Content)
		matrix.SendFormattedMessage(roomID, response)
	}
}

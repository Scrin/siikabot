package bot

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
}

type Choice struct {
	Message Message `json:"message"`
}

type ChatResponse struct {
	Choices []Choice `json:"choices"`
	Error   *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

var openrouterAPIKey string

func chat(roomID, sender, msg string) {
	if strings.TrimSpace(msg) == "" {
		return
	}

	req := ChatRequest{
		Model: "google/gemini-2.0-flash-lite-preview-02-05:free",
		Messages: []Message{
			{
				Role:    "user",
				Content: msg,
			},
		},
	}

	jsonData, err := json.Marshal(req)
	if err != nil {
		client.SendMessage(roomID, fmt.Sprintf("Error marshaling request: %v", err))
		return
	}

	httpReq, err := http.NewRequest("POST", "https://openrouter.ai/api/v1/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		client.SendMessage(roomID, fmt.Sprintf("Error creating request: %v", err))
		return
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+openrouterAPIKey)

	httpClient := &http.Client{}
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		client.SendMessage(roomID, fmt.Sprintf("Error making request: %v", err))
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		client.SendMessage(roomID, fmt.Sprintf("Error reading response: %v", err))
		return
	}

	var chatResp ChatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		client.SendMessage(roomID, fmt.Sprintf("Error parsing response: %v", err))
		return
	}

	if chatResp.Error != nil {
		client.SendMessage(roomID, fmt.Sprintf("API Error: %s (Type: %s)", chatResp.Error.Message, chatResp.Error.Type))
		return
	}

	if len(chatResp.Choices) > 0 {
		response := fmt.Sprintf("<a href=\"https://matrix.to/#/%s\">%s</a>: %s", sender, sender, chatResp.Choices[0].Message.Content)
		client.SendFormattedMessage(roomID, response)
	}
}

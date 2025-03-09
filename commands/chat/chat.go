package chat

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Scrin/siikabot/config"
	"github.com/Scrin/siikabot/db"
	"github.com/Scrin/siikabot/matrix"
	"github.com/Scrin/siikabot/metrics"
	"github.com/Scrin/siikabot/openrouter"
	"github.com/rs/zerolog/log"
)

// Maximum number of previous messages to include in the conversation history
const maxHistoryMessages = 20

// How long to keep chat history before cleaning it up
const chatHistoryRetention = 7 * 24 * time.Hour // 7 days

// toolRegistry holds all available tools
var toolRegistry *openrouter.ToolRegistry

// Init initializes the chat module
func Init(ctx context.Context) {
	// Initialize the tool registry
	toolRegistry = openrouter.NewToolRegistry()

	// Register the tool implementations from the chat package
	toolRegistry.RegisterTool(ElectricityPricesToolDefinition)
	toolRegistry.RegisterTool(WeatherToolDefinition)
	toolRegistry.RegisterTool(WeatherForecastToolDefinition)
	toolRegistry.RegisterTool(NewsToolDefinition)
	toolRegistry.RegisterTool(WebSearchToolDefinition)

	// Start a goroutine to periodically clean up old chat history
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				cleanupChatHistory(ctx)
			}
		}
	}()
}

// cleanupChatHistory removes old chat history entries
func cleanupChatHistory(ctx context.Context) {
	count, err := db.CleanupOldChatHistory(ctx, chatHistoryRetention)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to clean up old chat history")
		return
	}
	if count > 0 {
		log.Info().Ctx(ctx).Int64("removed_count", count).Msg("Cleaned up old chat history")
	}
}

func Handle(ctx context.Context, roomID, sender, msg string) {
	split := strings.Split(msg, " ")
	if len(split) < 2 {
		return
	}

	switch strings.TrimSpace(split[1]) {
	case "reset":
		count, err := db.DeleteChatHistoryForRoom(ctx, roomID)
		if err != nil {
			log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Msg("Failed to reset chat history")
			matrix.SendMessage(roomID, "Failed to reset chat history")
			return
		}
		log.Info().Ctx(ctx).Str("room_id", roomID).Int64("deleted_count", count).Msg("Chat history reset")
		matrix.SendMessage(roomID, fmt.Sprintf("Chat history reset (%d messages deleted)", count))
	default:
		matrix.SendMessage(roomID, "Unknown command")
	}
}

// HandleMention handles the chat command
func HandleMention(ctx context.Context, roomID, sender, msg, eventID string) {
	if strings.TrimSpace(msg) == "" {
		return
	}

	// Mark the message as read
	matrix.MarkRead(ctx, roomID, eventID)

	log.Debug().Ctx(ctx).
		Str("room_id", roomID).
		Str("sender", sender).
		Str("chat_msg", msg).
		Msg("Processing chat command")

	// Send typing indicator to let the user know we're processing their request
	// Set a timeout that's long enough to cover the expected processing time
	matrix.SendTyping(ctx, roomID, true, 60*time.Second)
	// Make sure we stop the typing indicator when we're done
	defer matrix.SendTyping(ctx, roomID, false, 0)

	// Create a system prompt with bot identity and current time
	currentTime := time.Now().Format("Monday, January 2, 2006 15:04:05 MST")

	// Get the bot's actual display name from the Matrix server
	botDisplayName := matrix.GetDisplayName(ctx, config.UserID)
	if botDisplayName == "" {
		// Fallback to user ID if display name can't be retrieved
		botDisplayName = strings.Split(config.UserID, ":")[0][1:] // Remove @ and domain part
	}

	systemPrompt := fmt.Sprintf(
		"You are %s, a helpful Matrix bot. The current date and time is %s. "+
			"Keep your responses concise and helpful. Use markdown formatting in your responses.",
		botDisplayName,
		currentTime,
	)

	// Get conversation history
	history, err := db.GetChatHistory(ctx, roomID, maxHistoryMessages)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Msg("Failed to get chat history")
		// Continue without history if there's an error
	}

	// Build messages array with system prompt, history, and current message
	messages := []openrouter.Message{{Role: "system", Content: systemPrompt}}

	// Add conversation history
	for _, historyMsg := range history {
		messages = append(messages, openrouter.Message{
			Role:    historyMsg.Role,
			Content: historyMsg.Message,
		})
	}

	// Add the current message
	messages = append(messages, openrouter.Message{Role: "user", Content: msg})

	// Get tool definitions from the registry
	tools := toolRegistry.GetToolDefinitions()

	req := openrouter.ChatRequest{
		Model:    "openai/gpt-4o-mini",
		Messages: messages,
		Tools:    tools,
	}

	// Send the request to OpenRouter
	chatResp, err := openrouter.SendChatRequest(ctx, req)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to send chat request")
		matrix.SendTyping(ctx, roomID, false, 0) // Stop typing indicator on error
		matrix.SendMessage(roomID, "Failed to process chat request")
		return
	}

	if len(chatResp.Choices) == 0 {
		log.Error().Ctx(ctx).Msg("Chat API returned no choices")
		matrix.SendTyping(ctx, roomID, false, 0) // Stop typing indicator on error
		matrix.SendMessage(roomID, "No response from chat API")
		return
	}

	// Save the user message to history
	if err := db.SaveChatMessage(ctx, roomID, sender, msg, "user"); err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Msg("Failed to save user message to history")
		// Continue even if saving fails
	}

	// Get the assistant's response
	assistantResponse := chatResp.Choices[0].Message.Content

	// Record character counts
	inputChars := len(msg)
	outputChars := len(assistantResponse)
	metrics.RecordChatCharacters(req.Model, inputChars, outputChars)

	// Check if the model wants to use a tool
	if chatResp.Choices[0].FinishReason == "tool_calls" && len(chatResp.Choices[0].Message.ToolCalls) > 0 {
		log.Debug().Ctx(ctx).
			Str("room_id", roomID).
			Str("sender", sender).
			Int("tool_calls", len(chatResp.Choices[0].Message.ToolCalls)).
			Msg("Model requested tool calls")

		// Add the tool response to the conversation
		// First, add the assistant's message with tool calls
		messages = append(messages, openrouter.Message{
			Role:      "assistant",
			Content:   "", // Content should be empty when there are tool calls
			ToolCalls: chatResp.Choices[0].Message.ToolCalls,
		})

		// Then add individual tool responses for each tool call
		toolResponses, err := toolRegistry.HandleToolCallsIndividually(ctx, chatResp.Choices[0].Message.ToolCalls)
		if err != nil {
			log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Msg("Failed to handle tool calls")
			matrix.SendMessage(roomID, "Failed to process tool calls")
			return
		}

		// Add each tool response as a separate message
		for _, toolResp := range toolResponses {
			messages = append(messages, openrouter.Message{
				Role:       "tool",
				Content:    toolResp.Response,
				ToolCallID: toolResp.ToolCallID,
			})
		}

		// Make a second request to get the final response, using the same model
		req.Messages = messages
		req.Tools = nil // No need for tools in the second request

		// Send typing indicator again for the second request
		matrix.SendTyping(ctx, roomID, true, 30*time.Second)

		// Log the request for debugging
		log.Debug().Ctx(ctx).
			Str("room_id", roomID).
			Str("sender", sender).
			Msg("Sending second chat request")

		// Send the second request to OpenRouter
		secondChatResp, err := openrouter.SendChatRequest(ctx, req)
		if err != nil {
			log.Error().Ctx(ctx).Err(err).Msg("Failed to send second chat request")
			matrix.SendTyping(ctx, roomID, false, 0) // Stop typing indicator on error
			matrix.SendMessage(roomID, "Failed to get response from chat API")
			return
		} else if len(secondChatResp.Choices) == 0 {
			log.Error().Ctx(ctx).Msg("Second chat API returned no choices")
			matrix.SendTyping(ctx, roomID, false, 0) // Stop typing indicator on error
			matrix.SendMessage(roomID, "No response from chat API")
			return
		} else {
			// Record character counts for the second call
			secondInputChars := 0
			for _, resp := range toolResponses {
				secondInputChars += len(resp.Response)
			}
			secondOutputChars := len(secondChatResp.Choices[0].Message.Content)
			metrics.RecordChatCharacters(req.Model, secondInputChars, secondOutputChars)

			assistantResponse = secondChatResp.Choices[0].Message.Content
		}
	}

	// Save the assistant response to history
	if err := db.SaveChatMessage(ctx, roomID, config.UserID, assistantResponse, "assistant"); err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Msg("Failed to save assistant message to history")
		// Continue even if saving fails
	}

	log.Debug().Ctx(ctx).
		Str("room_id", roomID).
		Str("sender", sender).
		Int("response_length", len(assistantResponse)).
		Msg("Chat command completed")

	matrix.SendMarkdownFormattedNotice(roomID, assistantResponse)
}

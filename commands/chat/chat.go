package chat

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Scrin/siikabot/config"
	"github.com/Scrin/siikabot/db"
	"github.com/Scrin/siikabot/matrix"
	"github.com/Scrin/siikabot/openrouter"
	"github.com/rs/zerolog/log"
)

const defaultModel = "openai/gpt-4o-mini"

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
	toolRegistry.RegisterTool(ReminderToolDefinition)
	toolRegistry.RegisterTool(GitHubIssueToolDefinition)

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

// getTextModelForRoom returns the model to use for text messages in a specific room
// If no room-specific model is set, returns the default model
func getTextModelForRoom(ctx context.Context, roomID string) string {
	model, err := db.GetRoomChatLLMModelText(ctx, roomID)
	if err != nil || model == "" {
		return defaultModel
	}
	return model
}

// getImageModelForRoom returns the model to use for image messages in a specific room
// If no room-specific model is set, returns the default model
func getImageModelForRoom(ctx context.Context, roomID string) string {
	model, err := db.GetRoomChatLLMModelImage(ctx, roomID)
	if err != nil || model == "" {
		return defaultModel
	}
	return model
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
	case "config":
		// Show current configuration for the room
		textModel := getTextModelForRoom(ctx, roomID)
		imageModel := getImageModelForRoom(ctx, roomID)
		matrix.SendMessage(roomID, fmt.Sprintf("Current chat configuration for this room:\nText model: %s\nImage model: %s", textModel, imageModel))
	case "model":
		if len(split) < 4 {
			matrix.SendMessage(roomID, "Usage: !chat model [text|image] <model_name>")
			return
		}

		if sender != config.Admin {
			matrix.SendMessage(roomID, "Only admins can change the chat models")
			return
		}

		modelType := strings.TrimSpace(split[2])
		newModel := strings.TrimSpace(split[3])

		switch modelType {
		case "text":
			err := db.SetRoomChatLLMModelText(ctx, roomID, newModel)
			if err != nil {
				log.Error().Ctx(ctx).Err(err).
					Str("room_id", roomID).
					Str("model", newModel).
					Msg("Failed to set room text chat model")
				matrix.SendMessage(roomID, "Failed to set text chat model")
				return
			}
			log.Info().Ctx(ctx).
				Str("room_id", roomID).
				Str("model", newModel).
				Msg("Text chat model changed")
			matrix.SendMessage(roomID, fmt.Sprintf("Text chat model changed to: %s", newModel))
		case "image":
			err := db.SetRoomChatLLMModelImage(ctx, roomID, newModel)
			if err != nil {
				log.Error().Ctx(ctx).Err(err).
					Str("room_id", roomID).
					Str("model", newModel).
					Msg("Failed to set room image chat model")
				matrix.SendMessage(roomID, "Failed to set image chat model")
				return
			}
			log.Info().Ctx(ctx).
				Str("room_id", roomID).
				Str("model", newModel).
				Msg("Image chat model changed")
			matrix.SendMessage(roomID, fmt.Sprintf("Image chat model changed to: %s", newModel))
		default:
			matrix.SendMessage(roomID, "Usage: !chat model [text|image] <model_name>")
		}

	default:
		matrix.SendMessage(roomID, "Unknown command")
	}
}

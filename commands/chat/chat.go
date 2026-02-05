package chat

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Scrin/siikabot/config"
	"github.com/Scrin/siikabot/db"
	"github.com/Scrin/siikabot/llmtools"
	"github.com/Scrin/siikabot/matrix"
	"github.com/Scrin/siikabot/openrouter"
	"github.com/rs/zerolog/log"
)

const defaultModel = "openai/gpt-4o-mini"

// Default values for configurable parameters
const defaultMaxHistoryMessages = 20
const defaultMaxToolIterations = 5

// How long to keep chat history before cleaning it up
const chatHistoryRetention = 7 * 24 * time.Hour // 7 days

// toolRegistry holds all available tools
var toolRegistry *openrouter.ToolRegistry

// Init initializes the chat module
func Init(ctx context.Context) {
	// Initialize the tool registry
	toolRegistry = openrouter.NewToolRegistry()

	// Register the tool implementations from the chat package
	toolRegistry.RegisterTool(llmtools.ElectricityPricesToolDefinition)
	toolRegistry.RegisterTool(llmtools.WeatherToolDefinition)
	toolRegistry.RegisterTool(llmtools.WeatherForecastToolDefinition)
	toolRegistry.RegisterTool(llmtools.NewsToolDefinition)
	toolRegistry.RegisterTool(llmtools.WebSearchToolDefinition)
	toolRegistry.RegisterTool(llmtools.ReminderToolDefinition)
	toolRegistry.RegisterTool(llmtools.GitHubIssueToolDefinition)
	toolRegistry.RegisterTool(llmtools.FingridToolDefinition)
	toolRegistry.RegisterTool(llmtools.WebToolDefinition)
	toolRegistry.RegisterTool(llmtools.WhoisToolDefinition)
	toolRegistry.RegisterTool(llmtools.GitHubStatusToolDefinition)
	toolRegistry.RegisterTool(llmtools.DNSToolDefinition)
	toolRegistry.RegisterTool(llmtools.ExchangeRatesToolDefinition)
	toolRegistry.RegisterTool(llmtools.TimezoneToolDefinition)
	toolRegistry.RegisterTool(llmtools.WikipediaToolDefinition)
	toolRegistry.RegisterTool(llmtools.MemoryToolDefinition)
	toolRegistry.RegisterTool(llmtools.UserGrafanaToolDefinition)

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
	if err != nil || model == nil {
		return defaultModel
	}
	return *model
}

// getImageModelForRoom returns the model to use for image messages in a specific room
// If no room-specific model is set, returns the default model
func getImageModelForRoom(ctx context.Context, roomID string) string {
	model, err := db.GetRoomChatLLMModelImage(ctx, roomID)
	if err != nil || model == nil {
		return defaultModel
	}
	return *model
}

// getMaxHistoryMessagesForRoom returns the max history messages to use for a specific room
// If no room-specific value is set, returns the default value
func getMaxHistoryMessagesForRoom(ctx context.Context, roomID string) int {
	maxMessages, err := db.GetRoomChatMaxHistoryMessages(ctx, roomID)
	if err != nil || maxMessages == nil {
		return defaultMaxHistoryMessages
	}
	return *maxMessages
}

// getMaxToolIterationsForRoom returns the max tool iterations to use for a specific room
// If no room-specific value is set, returns the default value
func getMaxToolIterationsForRoom(ctx context.Context, roomID string) int {
	maxIterations, err := db.GetRoomChatMaxToolIterations(ctx, roomID)
	if err != nil || maxIterations == nil {
		return defaultMaxToolIterations
	}
	return *maxIterations
}

// getMaxWebContentSizeForRoom returns the max web content size to use for a specific room
// If no room-specific value is set, returns the default value
func getMaxWebContentSizeForRoom(ctx context.Context, roomID string) int {
	maxSize, err := db.GetRoomChatMaxWebContentSize(ctx, roomID)
	if err != nil || maxSize == nil {
		return llmtools.DefaultMaxWebResponseSize
	}
	return *maxSize
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
		maxHistoryMessages := getMaxHistoryMessagesForRoom(ctx, roomID)
		maxToolIterations := getMaxToolIterationsForRoom(ctx, roomID)
		maxWebContentSize := getMaxWebContentSizeForRoom(ctx, roomID)

		matrix.SendMessage(roomID, fmt.Sprintf("Current chat configuration for this room:\n"+
			"Text model: %s\n"+
			"Image model: %s\n"+
			"Max history messages: %d\n"+
			"Max tool iterations: %d\n"+
			"Max web content size: %d bytes",
			textModel, imageModel, maxHistoryMessages, maxToolIterations, maxWebContentSize))
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
	case "history":
		if len(split) < 3 {
			matrix.SendMessage(roomID, "Usage: !chat history <max_messages>")
			return
		}

		if sender != config.Admin {
			matrix.SendMessage(roomID, "Only admins can change the max history messages")
			return
		}

		var maxMessages int
		_, err := fmt.Sscanf(split[2], "%d", &maxMessages)
		if err != nil || maxMessages <= 0 {
			matrix.SendMessage(roomID, "Max messages must be a positive integer")
			return
		}

		err = db.SetRoomChatMaxHistoryMessages(ctx, roomID, maxMessages)
		if err != nil {
			log.Error().Ctx(ctx).Err(err).
				Str("room_id", roomID).
				Int("max_messages", maxMessages).
				Msg("Failed to set room max history messages")
			matrix.SendMessage(roomID, "Failed to set max history messages")
			return
		}
		log.Info().Ctx(ctx).
			Str("room_id", roomID).
			Int("max_messages", maxMessages).
			Msg("Max history messages changed")
		matrix.SendMessage(roomID, fmt.Sprintf("Max history messages changed to: %d", maxMessages))
	case "tools":
		if len(split) < 3 {
			matrix.SendMessage(roomID, "Usage: !chat tools <max_iterations>")
			return
		}

		if sender != config.Admin {
			matrix.SendMessage(roomID, "Only admins can change the max tool iterations")
			return
		}

		var maxIterations int
		_, err := fmt.Sscanf(split[2], "%d", &maxIterations)
		if err != nil || maxIterations <= 0 {
			matrix.SendMessage(roomID, "Max iterations must be a positive integer")
			return
		}

		err = db.SetRoomChatMaxToolIterations(ctx, roomID, maxIterations)
		if err != nil {
			log.Error().Ctx(ctx).Err(err).
				Str("room_id", roomID).
				Int("max_iterations", maxIterations).
				Msg("Failed to set room max tool iterations")
			matrix.SendMessage(roomID, "Failed to set max tool iterations")
			return
		}
		log.Info().Ctx(ctx).
			Str("room_id", roomID).
			Int("max_iterations", maxIterations).
			Msg("Max tool iterations changed")
		matrix.SendMessage(roomID, fmt.Sprintf("Max tool iterations changed to: %d", maxIterations))
	case "web":
		if len(split) < 3 {
			matrix.SendMessage(roomID, "Usage: !chat web <max_size_bytes>")
			return
		}

		if sender != config.Admin {
			matrix.SendMessage(roomID, "Only admins can change the max web content size")
			return
		}

		var maxSize int
		_, err := fmt.Sscanf(split[2], "%d", &maxSize)
		if err != nil || maxSize <= 0 {
			matrix.SendMessage(roomID, "Max web content size must be a positive integer")
			return
		}

		err = db.SetRoomChatMaxWebContentSize(ctx, roomID, maxSize)
		if err != nil {
			log.Error().Ctx(ctx).Err(err).
				Str("room_id", roomID).
				Int("max_size", maxSize).
				Msg("Failed to set room max web content size")
			matrix.SendMessage(roomID, "Failed to set max web content size")
			return
		}
		log.Info().Ctx(ctx).
			Str("room_id", roomID).
			Int("max_size", maxSize).
			Msg("Max web content size changed")
		matrix.SendMessage(roomID, fmt.Sprintf("Max web content size changed to: %d bytes", maxSize))
	default:
		matrix.SendMessage(roomID, "Unknown command")
	}
}

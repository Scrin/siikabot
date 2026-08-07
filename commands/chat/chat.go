package chat

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Scrin/siikabot/aigateway"
	"github.com/Scrin/siikabot/config"
	"github.com/Scrin/siikabot/db"
	"github.com/Scrin/siikabot/llmtools"
	"github.com/Scrin/siikabot/matrix"
	"github.com/rs/zerolog/log"
)

const defaultModel = "openai/gpt-4o-mini"

// Default values for configurable parameters
const defaultMaxToolIterations = 5

// defaultMaxTokens caps a single response. Generous enough that ordinary answers are unaffected —
// the system prompt already asks for concise replies — but a backstop so a runaway generation is
// not billed in full. Raise it per room with "!chat maxtokens" if replies get cut short.
const defaultMaxTokens = 2048

// defaultImageDetail is the fidelity images are sent at. "low" costs a flat, small number of tokens
// per image, where the provider default tiles the image and can cost thousands for the same picture.
const defaultImageDetail = "low"

// imageDetailAuto omits the detail field, leaving the choice to the provider
const imageDetailAuto = "auto"

// How long to keep chat history before cleaning it up
const chatHistoryRetention = 7 * 24 * time.Hour // 7 days

// toolRegistry holds all available tools
var toolRegistry *aigateway.ToolRegistry

// Init initializes the chat module
func Init(ctx context.Context) {
	// Initialize the tool registry
	toolRegistry = aigateway.NewToolRegistry()

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

// getMaxTokensForRoom returns the response length cap for a room
// If no room-specific value is set, returns the default value
func getMaxTokensForRoom(ctx context.Context, roomID string) int {
	maxTokens, err := db.GetRoomChatMaxTokens(ctx, roomID)
	if err != nil || maxTokens == nil || *maxTokens <= 0 {
		return defaultMaxTokens
	}
	return *maxTokens
}

// getImageDetailForRoom returns the image detail level for a room.
// Returns an empty string for "auto", which omits the field and lets the provider choose.
func getImageDetailForRoom(ctx context.Context, roomID string) string {
	detail, err := db.GetRoomChatImageDetail(ctx, roomID)
	if err != nil || detail == nil || *detail == "" {
		return defaultImageDetail
	}
	if *detail == imageDetailAuto {
		return ""
	}
	return *detail
}

// describeImageDetail renders the room's image detail setting for display
func describeImageDetail(ctx context.Context, roomID string) string {
	if detail := getImageDetailForRoom(ctx, roomID); detail != "" {
		return detail
	}
	return imageDetailAuto
}

// describeContextWindow renders the room's context window settings and the size of the window as it
// currently stands, since a token budget on its own is hard to picture.
//
// Deliberately reads the window without maintaining it: showing the configuration should not move
// the anchor as a side effect.
func describeContextWindow(ctx context.Context, roomID string) string {
	high, low := getContextBudgetForRoom(ctx, roomID)
	window := currentContextWindow(ctx, roomID)
	return fmt.Sprintf("%d / %d tokens (currently ~%d tokens over %d messages)",
		high, low, estimateHistoryTokens(window), len(window))
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
		// The anchor points at a row that no longer exists, so clear it along with the history
		if err := db.ClearRoomChatContextAnchor(ctx, roomID); err != nil {
			log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Msg("Failed to clear context anchor on reset")
			// Continue: a stale anchor degrades to starting from the oldest available row
		}
		log.Info().Ctx(ctx).Str("room_id", roomID).Int64("deleted_count", count).Msg("Chat history reset")
		matrix.SendMessage(roomID, fmt.Sprintf("Chat history reset (%d messages deleted)", count))
	case "config":
		// Show current configuration for the room
		textModel := getTextModelForRoom(ctx, roomID)
		imageModel := getImageModelForRoom(ctx, roomID)
		contextWindow := describeContextWindow(ctx, roomID)
		maxToolIterations := getMaxToolIterationsForRoom(ctx, roomID)
		maxWebContentSize := getMaxWebContentSizeForRoom(ctx, roomID)

		matrix.SendMessage(roomID, fmt.Sprintf("Current chat configuration for this room:\n"+
			"Text model: %s\n"+
			"Image model: %s\n"+
			"Context window: %s\n"+
			"Max response tokens: %d\n"+
			"Image detail: %s\n"+
			"Max tool iterations: %d\n"+
			"Max web content size: %d bytes",
			textModel, imageModel, contextWindow, getMaxTokensForRoom(ctx, roomID),
			describeImageDetail(ctx, roomID), maxToolIterations, maxWebContentSize))
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
	case "context":
		if len(split) < 4 {
			matrix.SendMessage(roomID, "Usage: !chat context <high_tokens> <low_tokens>")
			return
		}

		if sender != config.Admin {
			matrix.SendMessage(roomID, "Only admins can change the context window")
			return
		}

		var highTokens, lowTokens int
		if _, err := fmt.Sscanf(split[2], "%d", &highTokens); err != nil || highTokens <= 0 {
			matrix.SendMessage(roomID, "High mark must be a positive integer")
			return
		}
		if _, err := fmt.Sscanf(split[3], "%d", &lowTokens); err != nil || lowTokens <= 0 {
			matrix.SendMessage(roomID, "Low mark must be a positive integer")
			return
		}
		// The window grows to the high mark then drops back to the low mark, so the marks have to
		// straddle a usable range or the window would be trimmed on every single turn
		if lowTokens >= highTokens {
			matrix.SendMessage(roomID, "Low mark must be below the high mark")
			return
		}

		if err := db.SetRoomChatContextTokens(ctx, roomID, highTokens, lowTokens); err != nil {
			log.Error().Ctx(ctx).Err(err).
				Str("room_id", roomID).
				Int("high_tokens", highTokens).
				Int("low_tokens", lowTokens).
				Msg("Failed to set room context window")
			matrix.SendMessage(roomID, "Failed to set context window")
			return
		}
		log.Info().Ctx(ctx).
			Str("room_id", roomID).
			Int("high_tokens", highTokens).
			Int("low_tokens", lowTokens).
			Msg("Context window changed")
		matrix.SendMessage(roomID, fmt.Sprintf("Context window changed to: %d / %d tokens", highTokens, lowTokens))
	case "maxtokens":
		if len(split) < 3 {
			matrix.SendMessage(roomID, "Usage: !chat maxtokens <tokens>")
			return
		}

		if sender != config.Admin {
			matrix.SendMessage(roomID, "Only admins can change the max response tokens")
			return
		}

		var maxTokens int
		if _, err := fmt.Sscanf(split[2], "%d", &maxTokens); err != nil || maxTokens <= 0 {
			matrix.SendMessage(roomID, "Max response tokens must be a positive integer")
			return
		}

		if err := db.SetRoomChatMaxTokens(ctx, roomID, maxTokens); err != nil {
			log.Error().Ctx(ctx).Err(err).
				Str("room_id", roomID).
				Int("max_tokens", maxTokens).
				Msg("Failed to set room max response tokens")
			matrix.SendMessage(roomID, "Failed to set max response tokens")
			return
		}
		log.Info().Ctx(ctx).
			Str("room_id", roomID).
			Int("max_tokens", maxTokens).
			Msg("Max response tokens changed")
		matrix.SendMessage(roomID, fmt.Sprintf("Max response tokens changed to: %d", maxTokens))
	case "imagedetail":
		if len(split) < 3 {
			matrix.SendMessage(roomID, "Usage: !chat imagedetail [low|high|auto]")
			return
		}

		if sender != config.Admin {
			matrix.SendMessage(roomID, "Only admins can change the image detail")
			return
		}

		detail := strings.TrimSpace(split[2])
		if detail != "low" && detail != "high" && detail != imageDetailAuto {
			matrix.SendMessage(roomID, "Usage: !chat imagedetail [low|high|auto]")
			return
		}

		if err := db.SetRoomChatImageDetail(ctx, roomID, detail); err != nil {
			log.Error().Ctx(ctx).Err(err).
				Str("room_id", roomID).
				Str("image_detail", detail).
				Msg("Failed to set room image detail")
			matrix.SendMessage(roomID, "Failed to set image detail")
			return
		}
		log.Info().Ctx(ctx).
			Str("room_id", roomID).
			Str("image_detail", detail).
			Msg("Image detail changed")
		matrix.SendMessage(roomID, fmt.Sprintf("Image detail changed to: %s", detail))
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

package chat

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Scrin/siikabot/tracing"

	"github.com/Scrin/siikabot/aigateway"
	"github.com/Scrin/siikabot/config"
	"github.com/Scrin/siikabot/db"
	"github.com/Scrin/siikabot/llmtools"
	"github.com/Scrin/siikabot/matrix"
	"github.com/rs/zerolog/log"
)

// imageDetailAuto omits the detail field, leaving the choice to the provider
const imageDetailAuto = "auto"

// requestImageDetail turns the configured image detail into the value sent with a request, where an
// empty value omits the field
func requestImageDetail(detail string) string {
	if detail == imageDetailAuto {
		return ""
	}
	return detail
}

// How long to keep chat history before cleaning it up
const chatHistoryRetention = 7 * 24 * time.Hour // 7 days

// How long to keep usage accounting records. Longer than the history itself: the conversations are
// transient, but the point of the accounting is to show cost trends over weeks.
const chatUsageRetention = 90 * 24 * time.Hour // 90 days

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

	// Start a goroutine to periodically clean up old chat history
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Each tick is its own root span: there is no request to inherit from, and
				// attaching to the bot's context would produce a span that never ends
				tracing.Run(ctx, tracer, "chat.cleanup", func(tickCtx context.Context) {
					cleanupChatHistory(tickCtx)
					cleanupChatUsage(tickCtx)
				})
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

// cleanupChatUsage removes old usage accounting records. Kept far longer than the chat history
// itself, since the point of the accounting is to show trends over weeks.
func cleanupChatUsage(ctx context.Context) {
	count, err := db.CleanupOldChatUsage(ctx, chatUsageRetention)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to clean up old chat usage")
		return
	}
	if count > 0 {
		log.Info().Ctx(ctx).Int64("removed_count", count).Msg("Cleaned up old chat usage")
	}
}

// describeContextWindow renders the context window settings and the size of the room's window as it
// currently stands, since a token budget on its own is hard to picture.
//
// Deliberately reads the window without maintaining it: showing the configuration should not move
// the anchor as a side effect.
func describeContextWindow(ctx context.Context, roomID string, cfg db.ChatConfig) string {
	window := currentContextWindow(ctx, roomID)
	return fmt.Sprintf("%d / %d tokens (currently ~%d tokens over %d messages)",
		cfg.ContextHighTokens, cfg.ContextLowTokens, estimateHistoryTokens(window), len(window))
}

// updateConfig applies a change to the chat configuration and reports the outcome to the room.
// Every change is recorded as a new configuration row, so the previous configuration stays on record.
func updateConfig(ctx context.Context, roomID string, change func(*db.ChatConfig), changedMsg, failedMsg string) {
	cfg, err := db.UpdateChatConfig(ctx, change)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Msg("Failed to update chat config")
		matrix.SendMessage(ctx, roomID, failedMsg)
		return
	}
	log.Info().Ctx(ctx).
		Str("room_id", roomID).
		Int64("config_id", cfg.ID).
		Str("text_model", cfg.TextModel).
		Str("image_model", cfg.ImageModel).
		Int("context_high_tokens", cfg.ContextHighTokens).
		Int("context_low_tokens", cfg.ContextLowTokens).
		Int("max_tokens", cfg.MaxTokens).
		Str("image_detail", cfg.ImageDetail).
		Int("max_tool_iterations", cfg.MaxToolIterations).
		Int("max_web_content_size", cfg.MaxWebContentSize).
		Msg("Chat config changed")
	matrix.SendMessage(ctx, roomID, changedMsg)
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
			matrix.SendMessage(ctx, roomID, "Failed to reset chat history")
			return
		}
		// The anchor points at a row that no longer exists, so clear it along with the history
		if err := db.ClearChatContextAnchor(ctx, roomID); err != nil {
			log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Msg("Failed to clear context anchor on reset")
			// Continue: a stale anchor degrades to starting from the oldest available row
		}
		log.Info().Ctx(ctx).Str("room_id", roomID).Int64("deleted_count", count).Msg("Chat history reset")
		matrix.SendMessage(ctx, roomID, fmt.Sprintf("Chat history reset (%d messages deleted)", count))
	case "config":
		// Show the current configuration
		cfg, err := db.GetChatConfig(ctx)
		if err != nil {
			log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Msg("Failed to load chat config")
			matrix.SendMessage(ctx, roomID, "Failed to load chat configuration")
			return
		}

		matrix.SendMessage(ctx, roomID, fmt.Sprintf("Current chat configuration:\n"+
			"Text model: %s\n"+
			"Image model: %s\n"+
			"Context window: %s\n"+
			"Max response tokens: %d\n"+
			"Image detail: %s\n"+
			"Max tool iterations: %d\n"+
			"Max web content size: %d bytes",
			cfg.TextModel, cfg.ImageModel, describeContextWindow(ctx, roomID, cfg), cfg.MaxTokens,
			cfg.ImageDetail, cfg.MaxToolIterations, cfg.MaxWebContentSize))
	case "model":
		if len(split) < 4 {
			matrix.SendMessage(ctx, roomID, "Usage: !chat model [text|image] <model_name>")
			return
		}

		if sender != config.Admin {
			matrix.SendMessage(ctx, roomID, "Only admins can change the chat models")
			return
		}

		modelType := strings.TrimSpace(split[2])
		newModel := strings.TrimSpace(split[3])
		if newModel == "" {
			matrix.SendMessage(ctx, roomID, "Usage: !chat model [text|image] <model_name>")
			return
		}

		switch modelType {
		case "text":
			updateConfig(ctx, roomID, func(cfg *db.ChatConfig) { cfg.TextModel = newModel },
				fmt.Sprintf("Text chat model changed to: %s", newModel), "Failed to set text chat model")
		case "image":
			updateConfig(ctx, roomID, func(cfg *db.ChatConfig) { cfg.ImageModel = newModel },
				fmt.Sprintf("Image chat model changed to: %s", newModel), "Failed to set image chat model")
		default:
			matrix.SendMessage(ctx, roomID, "Usage: !chat model [text|image] <model_name>")
		}
	case "context":
		if len(split) < 4 {
			matrix.SendMessage(ctx, roomID, "Usage: !chat context <high_tokens> <low_tokens>")
			return
		}

		if sender != config.Admin {
			matrix.SendMessage(ctx, roomID, "Only admins can change the context window")
			return
		}

		var highTokens, lowTokens int
		if _, err := fmt.Sscanf(split[2], "%d", &highTokens); err != nil || highTokens <= 0 {
			matrix.SendMessage(ctx, roomID, "High mark must be a positive integer")
			return
		}
		if _, err := fmt.Sscanf(split[3], "%d", &lowTokens); err != nil || lowTokens <= 0 {
			matrix.SendMessage(ctx, roomID, "Low mark must be a positive integer")
			return
		}
		// The window grows to the high mark then drops back to the low mark, so the marks have to
		// straddle a usable range or the window would be trimmed on every single turn
		if lowTokens >= highTokens {
			matrix.SendMessage(ctx, roomID, "Low mark must be below the high mark")
			return
		}

		updateConfig(ctx, roomID, func(cfg *db.ChatConfig) {
			cfg.ContextHighTokens = highTokens
			cfg.ContextLowTokens = lowTokens
		}, fmt.Sprintf("Context window changed to: %d / %d tokens", highTokens, lowTokens), "Failed to set context window")
	case "maxtokens":
		if len(split) < 3 {
			matrix.SendMessage(ctx, roomID, "Usage: !chat maxtokens <tokens>")
			return
		}

		if sender != config.Admin {
			matrix.SendMessage(ctx, roomID, "Only admins can change the max response tokens")
			return
		}

		var maxTokens int
		if _, err := fmt.Sscanf(split[2], "%d", &maxTokens); err != nil || maxTokens <= 0 {
			matrix.SendMessage(ctx, roomID, "Max response tokens must be a positive integer")
			return
		}

		updateConfig(ctx, roomID, func(cfg *db.ChatConfig) { cfg.MaxTokens = maxTokens },
			fmt.Sprintf("Max response tokens changed to: %d", maxTokens), "Failed to set max response tokens")
	case "imagedetail":
		if len(split) < 3 {
			matrix.SendMessage(ctx, roomID, "Usage: !chat imagedetail [low|high|auto]")
			return
		}

		if sender != config.Admin {
			matrix.SendMessage(ctx, roomID, "Only admins can change the image detail")
			return
		}

		detail := strings.TrimSpace(split[2])
		if detail != "low" && detail != "high" && detail != imageDetailAuto {
			matrix.SendMessage(ctx, roomID, "Usage: !chat imagedetail [low|high|auto]")
			return
		}

		updateConfig(ctx, roomID, func(cfg *db.ChatConfig) { cfg.ImageDetail = detail },
			fmt.Sprintf("Image detail changed to: %s", detail), "Failed to set image detail")
	case "tools":
		if len(split) < 3 {
			matrix.SendMessage(ctx, roomID, "Usage: !chat tools <max_iterations>")
			return
		}

		if sender != config.Admin {
			matrix.SendMessage(ctx, roomID, "Only admins can change the max tool iterations")
			return
		}

		var maxIterations int
		_, err := fmt.Sscanf(split[2], "%d", &maxIterations)
		if err != nil || maxIterations <= 0 {
			matrix.SendMessage(ctx, roomID, "Max iterations must be a positive integer")
			return
		}

		updateConfig(ctx, roomID, func(cfg *db.ChatConfig) { cfg.MaxToolIterations = maxIterations },
			fmt.Sprintf("Max tool iterations changed to: %d", maxIterations), "Failed to set max tool iterations")
	case "web":
		if len(split) < 3 {
			matrix.SendMessage(ctx, roomID, "Usage: !chat web <max_size_bytes>")
			return
		}

		if sender != config.Admin {
			matrix.SendMessage(ctx, roomID, "Only admins can change the max web content size")
			return
		}

		var maxSize int
		_, err := fmt.Sscanf(split[2], "%d", &maxSize)
		if err != nil || maxSize <= 0 {
			matrix.SendMessage(ctx, roomID, "Max web content size must be a positive integer")
			return
		}

		updateConfig(ctx, roomID, func(cfg *db.ChatConfig) { cfg.MaxWebContentSize = maxSize },
			fmt.Sprintf("Max web content size changed to: %d bytes", maxSize), "Failed to set max web content size")
	default:
		matrix.SendMessage(ctx, roomID, "Unknown command")
	}
}

package llmtools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Scrin/siikabot/db"
	"github.com/Scrin/siikabot/openrouter"
	"github.com/rs/zerolog/log"
)

const maxMemoryLength = 500

// MemoryToolDefinition returns the tool definition for the memory tool
var MemoryToolDefinition = openrouter.ToolDefinition{
	Type: "function",
	Function: openrouter.FunctionSchema{
		Name:        "memory",
		Description: "Manage memories about the user. Use this to save things the user asks you to remember, delete specific memories, or clear all memories. Memories persist across conversations and rooms.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"action": {
					"type": "string",
					"enum": ["save", "delete", "clear_all"],
					"description": "The action to perform: 'save' to remember something new, 'delete' to remove a specific memory by ID, 'clear_all' to remove all memories"
				},
				"memory": {
					"type": "string",
					"description": "The memory to save (required for 'save' action, max 500 characters). Should be a concise fact or preference about the user."
				},
				"memory_id": {
					"type": "integer",
					"description": "The ID of the memory to delete (required for 'delete' action)"
				}
			},
			"required": ["action"]
		}`),
	},
	Handler: handleMemoryToolCall,
}

// handleMemoryToolCall handles memory tool calls
func handleMemoryToolCall(ctx context.Context, arguments string) (string, error) {
	var args struct {
		Action   string `json:"action"`
		Memory   string `json:"memory"`
		MemoryID *int64 `json:"memory_id"`
	}

	log.Debug().Ctx(ctx).Str("arguments", arguments).Msg("Received memory tool call")

	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		log.Error().Ctx(ctx).Err(err).Str("arguments", arguments).Msg("Failed to parse memory tool arguments")
		return "", fmt.Errorf("failed to parse arguments: %w", err)
	}

	// Get sender from context
	sender, ok := ctx.Value("sender").(string)
	if !ok || sender == "" {
		return "", errors.New("sender not found in context")
	}

	switch args.Action {
	case "save":
		return handleSaveMemory(ctx, sender, args.Memory)
	case "delete":
		return handleDeleteMemory(ctx, sender, args.MemoryID)
	case "clear_all":
		return handleClearAllMemories(ctx, sender)
	default:
		return "", fmt.Errorf("unknown action: %s", args.Action)
	}
}

func handleSaveMemory(ctx context.Context, userID, memory string) (string, error) {
	if memory == "" {
		return "", errors.New("memory is required for save action")
	}

	if len(memory) > maxMemoryLength {
		return "", fmt.Errorf("memory exceeds maximum length of %d characters", maxMemoryLength)
	}

	err := db.SaveMemory(ctx, userID, memory)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("user_id", userID).
			Msg("Failed to save memory")
		return "", fmt.Errorf("failed to save memory: %w", err)
	}

	log.Info().Ctx(ctx).
		Str("user_id", userID).
		Str("memory", memory).
		Msg("Memory saved")

	return "Memory saved successfully.", nil
}

func handleDeleteMemory(ctx context.Context, userID string, memoryID *int64) (string, error) {
	if memoryID == nil {
		return "", errors.New("memory_id is required for delete action")
	}

	err := db.DeleteMemory(ctx, userID, *memoryID)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("user_id", userID).
			Int64("memory_id", *memoryID).
			Msg("Failed to delete memory")
		return "", fmt.Errorf("failed to delete memory: %w", err)
	}

	log.Info().Ctx(ctx).
		Str("user_id", userID).
		Int64("memory_id", *memoryID).
		Msg("Memory deleted")

	return "Memory deleted successfully.", nil
}

func handleClearAllMemories(ctx context.Context, userID string) (string, error) {
	count, err := db.DeleteAllMemories(ctx, userID)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("user_id", userID).
			Msg("Failed to clear all memories")
		return "", fmt.Errorf("failed to clear all memories: %w", err)
	}

	log.Info().Ctx(ctx).
		Str("user_id", userID).
		Int64("deleted_count", count).
		Msg("All memories cleared")

	return fmt.Sprintf("All memories cleared (%d memories deleted).", count), nil
}

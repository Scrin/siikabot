package llmtools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Scrin/siikabot/aigateway"
	"github.com/Scrin/siikabot/db"
	"github.com/rs/zerolog/log"
)

const maxMemoryLength = 500

// MemoryToolDefinition returns the tool definition for the memory tool
var MemoryToolDefinition = aigateway.ToolDefinition{
	Type: "function",
	Function: aigateway.FunctionSchema{
		Name:        "memory",
		Description: "Manage memories about the author of the latest message; you can't save or change memories for anyone else. Use this to save things they ask you to remember, delete specific memories, or clear all memories. A memory saved in a group room is used in that room and in direct chats; one saved in a direct chat is used only in direct chats. In a group room, delete and clear_all only affect the memories saved in that room.",
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
					"description": "The memory to save (required for 'save' action, max 500 characters). Should be a concise fact or preference about the author of the latest message."
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

	// Get sender and room from context
	sender, ok := ctx.Value("sender").(string)
	if !ok || sender == "" {
		return "", errors.New("sender not found in context")
	}
	roomID, ok := ctx.Value("room_id").(string)
	if !ok || roomID == "" {
		return "", errors.New("room ID not found in context")
	}
	// A room not known to be a DM counts as a group, which keeps a memory to that room
	isDM, _ := ctx.Value("room_is_dm").(bool)
	view := db.MemoryView{RoomID: roomID, IsDM: isDM}

	switch args.Action {
	case "save":
		return handleSaveMemory(ctx, sender, args.Memory, view)
	case "delete":
		return handleDeleteMemory(ctx, sender, args.MemoryID, view)
	case "clear_all":
		return handleClearAllMemories(ctx, sender, view)
	default:
		return "", fmt.Errorf("unknown action: %s", args.Action)
	}
}

func handleSaveMemory(ctx context.Context, userID, memory string, view db.MemoryView) (string, error) {
	if memory == "" {
		return "", errors.New("memory is required for save action")
	}

	if len(memory) > maxMemoryLength {
		return "", fmt.Errorf("memory exceeds maximum length of %d characters", maxMemoryLength)
	}

	err := db.SaveMemory(ctx, userID, memory, view)
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

func handleDeleteMemory(ctx context.Context, userID string, memoryID *int64, view db.MemoryView) (string, error) {
	if memoryID == nil {
		return "", errors.New("memory_id is required for delete action")
	}

	deleted, err := db.DeleteMemoryIn(ctx, userID, *memoryID, view)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("user_id", userID).
			Int64("memory_id", *memoryID).
			Msg("Failed to delete memory")
		return "", fmt.Errorf("failed to delete memory: %w", err)
	}
	if !deleted {
		return "No memory with that ID is visible here, so nothing was deleted.", nil
	}

	log.Info().Ctx(ctx).
		Str("user_id", userID).
		Int64("memory_id", *memoryID).
		Msg("Memory deleted")

	return "Memory deleted successfully.", nil
}

func handleClearAllMemories(ctx context.Context, userID string, view db.MemoryView) (string, error) {
	count, err := db.DeleteAllMemoriesIn(ctx, userID, view)
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

	if !view.IsDM {
		return fmt.Sprintf("Memories saved in this room cleared (%d deleted). Memories saved elsewhere are not affected, and can be cleared in a direct chat.", count), nil
	}
	return fmt.Sprintf("All memories cleared (%d memories deleted).", count), nil
}

package db

import (
	"context"
	"time"

	pgx "github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
)

// ChatMessage represents a message in the chat history
type ChatMessage struct {
	ID          int64      `db:"id"`
	RoomID      string     `db:"room_id"`
	UserID      string     `db:"user_id"`
	Message     string     `db:"message"`
	Role        string     `db:"role"`
	Timestamp   time.Time  `db:"timestamp"`
	MessageType string     `db:"message_type"`
	ToolCallID  *string    `db:"tool_call_id"`
	ToolName    *string    `db:"tool_name"`
	Expiry      *time.Time `db:"expiry"`
}

// SaveChatMessage saves a chat message to the database
func SaveChatMessage(ctx context.Context, roomID, userID, message, role string) error {
	return saveChatMessageWithDetails(ctx, roomID, userID, message, role, "text", nil, nil, nil)
}

// SaveToolCall saves a tool call to the database with an optional expiry time
func SaveToolCall(ctx context.Context, roomID, userID, toolCallID, toolName, arguments string, validityDuration time.Duration) error {
	var expiry *time.Time
	if validityDuration > 0 {
		expiryTime := time.Now().Add(validityDuration)
		expiry = &expiryTime
	}
	return saveChatMessageWithDetails(ctx, roomID, userID, arguments, "assistant", "tool_call", &toolCallID, &toolName, expiry)
}

// SaveToolResponse saves a tool response to the database with an optional expiry time
func SaveToolResponse(ctx context.Context, roomID, userID, toolCallID, toolName, response string, validityDuration time.Duration) error {
	var expiry *time.Time
	if validityDuration > 0 {
		expiryTime := time.Now().Add(validityDuration)
		expiry = &expiryTime
	}
	return saveChatMessageWithDetails(ctx, roomID, userID, response, "tool", "tool_response", &toolCallID, &toolName, expiry)
}

// saveChatMessageWithDetails saves a chat message to the database with additional details
func saveChatMessageWithDetails(ctx context.Context, roomID, userID, message, role, messageType string, toolCallID, toolName *string, expiry *time.Time) error {
	_, err := pool.Exec(ctx,
		"INSERT INTO chat_history (room_id, user_id, message, role, message_type, tool_call_id, tool_name, expiry) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)",
		roomID, userID, message, role, messageType, toolCallID, toolName, expiry)
	if err != nil {
		toolCallIDStr := ""
		if toolCallID != nil {
			toolCallIDStr = *toolCallID
		}
		toolNameStr := ""
		if toolName != nil {
			toolNameStr = *toolName
		}
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", roomID).
			Str("user_id", userID).
			Str("role", role).
			Str("message_type", messageType).
			Str("tool_call_id", toolCallIDStr).
			Str("tool_name", toolNameStr).
			Msg("Failed to save chat message")
		return err
	}
	return nil
}

// GetChatHistory retrieves recent chat history for a room
// maxMessages is the maximum number of messages to retrieve
func GetChatHistory(ctx context.Context, roomID string, maxMessages int) ([]ChatMessage, error) {
	rows, err := pool.Query(ctx,
		`SELECT id, room_id, user_id, message, role, timestamp, message_type, tool_call_id, tool_name, expiry 
		FROM chat_history 
		WHERE room_id = $1 
		AND (expiry IS NULL OR expiry > NOW())
		ORDER BY timestamp DESC LIMIT $2`,
		roomID, maxMessages)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", roomID).
			Int("max_messages", maxMessages).
			Msg("Failed to get chat history")
		return nil, err
	}

	messages, err := pgx.CollectRows(rows, pgx.RowToStructByName[ChatMessage])
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", roomID).
			Msg("Failed to collect chat history rows")
		return nil, err
	}

	// Reverse the order to get chronological order (oldest first)
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	return messages, nil
}

// CleanupOldChatHistory removes chat messages older than the specified duration
func CleanupOldChatHistory(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoffTime := time.Now().Add(-olderThan)

	tag, err := pool.Exec(ctx,
		"DELETE FROM chat_history WHERE timestamp < $1",
		cutoffTime)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Time("cutoff_time", cutoffTime).
			Msg("Failed to cleanup old chat history")
		return 0, err
	}

	return tag.RowsAffected(), nil
}

// DeleteChatHistoryForRoom deletes all chat history for a specific room
func DeleteChatHistoryForRoom(ctx context.Context, roomID string) (int64, error) {
	tag, err := pool.Exec(ctx,
		"DELETE FROM chat_history WHERE room_id = $1",
		roomID)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", roomID).
			Msg("Failed to delete chat history for room")
		return 0, err
	}

	return tag.RowsAffected(), nil
}

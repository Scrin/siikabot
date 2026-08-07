package db

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	pgx "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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

// ToolCallRecord holds a tool call together with the response it produced, so the pair can be
// persisted atomically
type ToolCallRecord struct {
	ToolCallID       string
	ToolName         string
	Arguments        string
	Response         string
	ValidityDuration time.Duration
}

// SaveToolCallsWithResponses saves tool calls and their responses in a single transaction.
//
// The pairing must be atomic: a tool call persisted without its response leaves history that
// rebuilds into an assistant message carrying tool_calls with no tool reply, which the chat API
// rejects outright, breaking every later request in the room.
func SaveToolCallsWithResponses(ctx context.Context, roomID, userID string, records []ToolCallRecord) error {
	if len(records) == 0 {
		return nil
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", roomID).
			Int("record_count", len(records)).
			Msg("Failed to begin transaction for tool call history")
		return err
	}
	defer tx.Rollback(ctx)

	for _, record := range records {
		// The call and its response share an expiry so they always drop out of history together
		var expiry *time.Time
		if record.ValidityDuration > 0 {
			expiryTime := time.Now().Add(record.ValidityDuration)
			expiry = &expiryTime
		}

		if err := saveChatMessageTx(ctx, tx, roomID, userID, record.Arguments, "assistant", "tool_call",
			&record.ToolCallID, &record.ToolName, expiry); err != nil {
			return err
		}
		if err := saveChatMessageTx(ctx, tx, roomID, userID, record.Response, "tool", "tool_response",
			&record.ToolCallID, &record.ToolName, expiry); err != nil {
			return err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", roomID).
			Int("record_count", len(records)).
			Msg("Failed to commit tool call history")
		return err
	}

	return nil
}

// chatMessageExecutor is satisfied by both the connection pool and a transaction, so chat history
// rows can be written either standalone or as part of an atomic group
type chatMessageExecutor interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

// saveChatMessageWithDetails saves a chat message to the database with additional details
func saveChatMessageWithDetails(ctx context.Context, roomID, userID, message, role, messageType string, toolCallID, toolName *string, expiry *time.Time) error {
	return saveChatMessageTx(ctx, pool, roomID, userID, message, role, messageType, toolCallID, toolName, expiry)
}

// saveChatMessageTx saves a chat message using the given executor
func saveChatMessageTx(ctx context.Context, executor chatMessageExecutor, roomID, userID, message, role, messageType string, toolCallID, toolName *string, expiry *time.Time) error {
	// Ensure the message is valid UTF-8 and replace invalid sequences with a replacement character
	if !utf8.ValidString(message) {
		log.Warn().Ctx(ctx).
			Str("room_id", roomID).
			Str("user_id", userID).
			Str("role", role).
			Str("message_type", messageType).
			Msg("Message contains invalid UTF-8, cleaning up")
		message = strings.ToValidUTF8(message, "")
	}

	_, err := executor.Exec(ctx,
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
//
// Ordering is by timestamp with the id as a tiebreaker. The tiebreaker is required, not cosmetic:
// rows written inside one transaction all take the transaction start time from NOW(), so ordering
// by timestamp alone would return a tool call and its response in an arbitrary order.
func GetChatHistory(ctx context.Context, roomID string, maxMessages int) ([]ChatMessage, error) {
	rows, err := pool.Query(ctx,
		`SELECT id, room_id, user_id, message, role, timestamp, message_type, tool_call_id, tool_name, expiry 
		FROM chat_history
		WHERE room_id = $1
		AND (expiry IS NULL OR expiry > NOW())
		ORDER BY timestamp DESC, id DESC LIMIT $2`,
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

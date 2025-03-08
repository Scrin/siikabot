package db

import (
	"context"
	"time"

	pgx "github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
)

// ChatMessage represents a message in the chat history
type ChatMessage struct {
	ID        int64     `db:"id"`
	RoomID    string    `db:"room_id"`
	UserID    string    `db:"user_id"`
	Message   string    `db:"message"`
	Role      string    `db:"role"`
	Timestamp time.Time `db:"timestamp"`
}

// SaveChatMessage saves a chat message to the database
func SaveChatMessage(ctx context.Context, roomID, userID, message, role string) error {
	_, err := pool.Exec(ctx,
		"INSERT INTO chat_history (room_id, user_id, message, role) VALUES ($1, $2, $3, $4)",
		roomID, userID, message, role)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", roomID).
			Str("user_id", userID).
			Str("role", role).
			Msg("Failed to save chat message")
		return err
	}
	return nil
}

// GetChatHistory retrieves recent chat history for a room
// maxMessages is the maximum number of messages to retrieve
func GetChatHistory(ctx context.Context, roomID string, maxMessages int) ([]ChatMessage, error) {
	rows, err := pool.Query(ctx,
		"SELECT id, room_id, user_id, message, role, timestamp FROM chat_history WHERE room_id = $1 ORDER BY timestamp DESC LIMIT $2",
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

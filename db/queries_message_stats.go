package db

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
)

type MessageStats struct {
	RoomID         string    `db:"room_id"`
	UserID         string    `db:"user_id"`
	FirstSeen      time.Time `db:"first_seen"`
	LastSeen       time.Time `db:"last_seen"`
	MessageCount   int       `db:"message_count"`
	WordCount      int       `db:"word_count"`
	CharacterCount int       `db:"character_count"`
}

func UpdateMessageStats(ctx context.Context, roomID, userID, messageText string) error {
	wordCount := countWords(messageText)
	charCount := len(messageText)

	_, err := pool.Exec(ctx, `
		INSERT INTO message_stats (room_id, user_id, first_seen, last_seen, message_count, word_count, character_count)
		VALUES ($1, $2, NOW(), NOW(), 1, $3, $4)
		ON CONFLICT (room_id, user_id) DO UPDATE SET
			last_seen = NOW(),
			message_count = message_stats.message_count + 1,
			word_count = message_stats.word_count + $3,
			character_count = message_stats.character_count + $4
	`, roomID, userID, wordCount, charCount)

	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", roomID).
			Str("user_id", userID).
			Int("word_count", wordCount).
			Int("character_count", charCount).
			Msg("Failed to update message stats")
		return err
	}
	return nil
}

func GetMessageStats(ctx context.Context, roomID, userID string) (*MessageStats, error) {
	var stats MessageStats
	err := pool.QueryRow(ctx, `
		SELECT room_id, user_id, first_seen, last_seen, message_count, word_count, character_count
		FROM message_stats
		WHERE room_id = $1 AND user_id = $2
	`, roomID, userID).Scan(
		&stats.RoomID,
		&stats.UserID,
		&stats.FirstSeen,
		&stats.LastSeen,
		&stats.MessageCount,
		&stats.WordCount,
		&stats.CharacterCount,
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", roomID).
			Str("user_id", userID).
			Msg("Failed to get message stats")
		return nil, err
	}
	return &stats, nil
}

func countWords(s string) int {
	return len(strings.Fields(s))
}

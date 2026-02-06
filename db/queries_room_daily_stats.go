package db

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"
)

type RoomDailyStats struct {
	RoomID         string    `db:"room_id"`
	StatDate       time.Time `db:"stat_date"`
	MessageCount   int       `db:"message_count"`
	WordCount      int       `db:"word_count"`
	CharacterCount int       `db:"character_count"`
}

func UpdateRoomDailyStats(ctx context.Context, roomID, messageText string) error {
	wordCount := countWords(messageText)
	charCount := len(messageText)

	_, err := pool.Exec(ctx, `
		INSERT INTO room_daily_stats (room_id, stat_date, message_count, word_count, character_count)
		VALUES ($1, CURRENT_DATE, 1, $2, $3)
		ON CONFLICT (room_id, stat_date) DO UPDATE SET
			message_count = room_daily_stats.message_count + 1,
			word_count = room_daily_stats.word_count + $2,
			character_count = room_daily_stats.character_count + $3
	`, roomID, wordCount, charCount)

	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", roomID).
			Int("word_count", wordCount).
			Int("character_count", charCount).
			Msg("Failed to update room daily stats")
		return err
	}
	return nil
}

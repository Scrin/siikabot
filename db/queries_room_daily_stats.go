package db

import (
	"context"
	"sync"
	"time"

	"github.com/Scrin/siikabot/config"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
)

type RoomDailyStats struct {
	RoomID         string    `db:"room_id"`
	StatDate       time.Time `db:"stat_date"`
	MessageCount   int       `db:"message_count"`
	WordCount      int       `db:"word_count"`
	CharacterCount int       `db:"character_count"`
}

// statLocation is the bot's time zone, whose days the messages are counted by
var statLocation = sync.OnceValue(func() *time.Location {
	loc, err := time.LoadLocation(config.Timezone)
	if err != nil {
		return time.UTC
	}
	return loc
})

// StatDate is the day a message sent at the given time is counted on: its date in the bot's time
// zone, as midnight UTC on that date, which is how a stat_date reads back.
//
// The day used to be the database's CURRENT_DATE, which follows the database session's time zone
// rather than the bot's.
func StatDate(t time.Time) time.Time {
	return statDate(t, statLocation())
}

func statDate(t time.Time, loc *time.Location) time.Time {
	local := t.In(loc)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
}

func UpdateRoomDailyStats(ctx context.Context, roomID, messageText string) error {
	wordCount := countWords(messageText)
	charCount := countCharacters(messageText)
	day := StatDate(time.Now())

	_, err := pool.Exec(ctx, `
		INSERT INTO room_daily_stats (room_id, stat_date, message_count, word_count, character_count)
		VALUES ($1, $4, 1, $2, $3)
		ON CONFLICT (room_id, stat_date) DO UPDATE SET
			message_count = room_daily_stats.message_count + 1,
			word_count = room_daily_stats.word_count + $2,
			character_count = room_daily_stats.character_count + $3
	`, roomID, wordCount, charCount, day)

	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", roomID).
			Time("stat_date", day).
			Int("word_count", wordCount).
			Int("character_count", charCount).
			Msg("Failed to update room daily stats")
		return err
	}
	return nil
}

// GetRoomDailyStats returns a room's counts for the days from one date to another, both included,
// in date order. A day on which nothing was counted has no row.
func GetRoomDailyStats(ctx context.Context, roomID string, from, to time.Time) ([]RoomDailyStats, error) {
	rows, err := pool.Query(ctx, `
		SELECT room_id, stat_date, message_count, word_count, character_count
		FROM room_daily_stats
		WHERE room_id = $1 AND stat_date BETWEEN $2 AND $3
		ORDER BY stat_date
	`, roomID, from, to)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Time("from", from).Time("to", to).Msg("Failed to query room daily stats")
		return nil, err
	}
	days, err := pgx.CollectRows(rows, pgx.RowToStructByName[RoomDailyStats])
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Time("from", from).Time("to", to).Msg("Failed to read room daily stats")
		return nil, err
	}
	return days, nil
}

// GetFirstRoomStatDate returns the first day anything was counted on in a room, nil if nothing has
// been
func GetFirstRoomStatDate(ctx context.Context, roomID string) (*time.Time, error) {
	var first *time.Time
	err := pool.QueryRow(ctx, "SELECT MIN(stat_date) FROM room_daily_stats WHERE room_id = $1", roomID).Scan(&first)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Msg("Failed to get the first room stat date")
		return nil, err
	}
	return first, nil
}

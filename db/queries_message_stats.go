package db

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

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

// RankedMessageStats is someone's message stats in a room, with their place among everyone counted
// there
type RankedMessageStats struct {
	MessageStats
	// Rank is their place in the ranking. People with the same count share a place.
	Rank int `db:"rank"`
}

// RoomMessageTotals sums up the message stats of everyone counted in a room
type RoomMessageTotals struct {
	PeopleCount    int `db:"people_count"`
	MessageCount   int `db:"message_count"`
	WordCount      int `db:"word_count"`
	CharacterCount int `db:"character_count"`
	// CountedSince is when the earliest message counted in the room was sent, nil if none has been
	CountedSince *time.Time `db:"counted_since"`
}

// What the people of a room can be ranked by
const (
	RankByMessages = "messages"
	RankByWords    = "words"
)

func UpdateMessageStats(ctx context.Context, roomID, userID, messageText string) error {
	wordCount := countWords(messageText)
	charCount := countCharacters(messageText)

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

// GetRankedMessageStats returns the message stats of the given people in a room, each ranked by
// message count among everyone counted there. Someone with nothing counted in the room is left out.
func GetRankedMessageStats(ctx context.Context, roomID string, userIDs []string) ([]RankedMessageStats, error) {
	rows, err := pool.Query(ctx, `
		SELECT room_id, user_id, first_seen, last_seen, message_count, word_count, character_count, rank
		FROM (
			SELECT room_id, user_id, first_seen, last_seen, message_count, word_count, character_count,
				RANK() OVER (ORDER BY message_count DESC) AS rank
			FROM message_stats
			WHERE room_id = $1
		) ranked
		WHERE user_id = ANY($2)
	`, roomID, userIDs)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Strs("user_ids", userIDs).Msg("Failed to query ranked message stats")
		return nil, err
	}
	stats, err := pgx.CollectRows(rows, pgx.RowToStructByName[RankedMessageStats])
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Strs("user_ids", userIDs).Msg("Failed to read ranked message stats")
		return nil, err
	}
	return stats, nil
}

// GetTopMessageStats returns the people who have sent the most in a room, by messages or by words
// (RankByMessages or RankByWords), at most limit of them, most first
func GetTopMessageStats(ctx context.Context, roomID, rankBy string, limit int) ([]RankedMessageStats, error) {
	var byWords bool
	switch rankBy {
	case RankByMessages:
	case RankByWords:
		byWords = true
	default:
		return nil, fmt.Errorf("unknown ranking %q", rankBy)
	}

	rows, err := pool.Query(ctx, `
		SELECT room_id, user_id, first_seen, last_seen, message_count, word_count, character_count,
			RANK() OVER (ORDER BY score DESC) AS rank
		FROM (
			SELECT room_id, user_id, first_seen, last_seen, message_count, word_count, character_count,
				CASE WHEN $2 THEN word_count ELSE message_count END AS score
			FROM message_stats
			WHERE room_id = $1
		) scored
		ORDER BY score DESC, user_id
		LIMIT $3
	`, roomID, byWords, limit)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Str("rank_by", rankBy).Int("limit", limit).Msg("Failed to query top message stats")
		return nil, err
	}
	stats, err := pgx.CollectRows(rows, pgx.RowToStructByName[RankedMessageStats])
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Str("rank_by", rankBy).Int("limit", limit).Msg("Failed to read top message stats")
		return nil, err
	}
	return stats, nil
}

// GetRoomMessageTotals sums up the message stats of everyone counted in a room
func GetRoomMessageTotals(ctx context.Context, roomID string) (RoomMessageTotals, error) {
	rows, err := pool.Query(ctx, `
		SELECT COUNT(*) AS people_count,
			COALESCE(SUM(message_count), 0) AS message_count,
			COALESCE(SUM(word_count), 0) AS word_count,
			COALESCE(SUM(character_count), 0) AS character_count,
			MIN(first_seen) AS counted_since
		FROM message_stats
		WHERE room_id = $1
	`, roomID)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Msg("Failed to query room message totals")
		return RoomMessageTotals{}, err
	}
	totals, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[RoomMessageTotals])
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Msg("Failed to read room message totals")
		return RoomMessageTotals{}, err
	}
	return totals, nil
}

func countWords(s string) int {
	return len(strings.Fields(s))
}

// countCharacters counts the characters of a message, not its bytes: ä is one character but two
// bytes
func countCharacters(s string) int {
	return utf8.RuneCountInString(s)
}

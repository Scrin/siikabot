package db

import (
	"context"
	"time"

	pgx "github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
)

// UserMemory represents a memory stored for a user
type UserMemory struct {
	ID        int64     `db:"id"`
	UserID    string    `db:"user_id"`
	Memory    string    `db:"memory"`
	CreatedAt time.Time `db:"created_at"`
}

// SaveMemory saves a new memory for a user
func SaveMemory(ctx context.Context, userID, memory string) error {
	_, err := pool.Exec(ctx,
		"INSERT INTO user_memory (user_id, memory) VALUES ($1, $2)",
		userID, memory)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("user_id", userID).
			Msg("Failed to save user memory")
		return err
	}
	return nil
}

// GetUserMemories returns the 100 most recent memories for a user
func GetUserMemories(ctx context.Context, userID string) ([]UserMemory, error) {
	rows, err := pool.Query(ctx,
		"SELECT id, user_id, memory, created_at FROM user_memory WHERE user_id = $1 ORDER BY created_at DESC LIMIT 100",
		userID)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("user_id", userID).Msg("Failed to query user memories")
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[UserMemory])
}

// DeleteMemory deletes a specific memory for a user
func DeleteMemory(ctx context.Context, userID string, memoryID int64) error {
	result, err := pool.Exec(ctx,
		"DELETE FROM user_memory WHERE id = $1 AND user_id = $2",
		memoryID, userID)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("user_id", userID).
			Int64("memory_id", memoryID).
			Msg("Failed to delete user memory")
		return err
	}
	if result.RowsAffected() == 0 {
		log.Warn().Ctx(ctx).
			Str("user_id", userID).
			Int64("memory_id", memoryID).
			Msg("No memory found to delete")
	}
	return nil
}

// DeleteAllMemories deletes all memories for a user
func DeleteAllMemories(ctx context.Context, userID string) (int64, error) {
	result, err := pool.Exec(ctx,
		"DELETE FROM user_memory WHERE user_id = $1",
		userID)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("user_id", userID).
			Msg("Failed to delete all user memories")
		return 0, err
	}
	return result.RowsAffected(), nil
}

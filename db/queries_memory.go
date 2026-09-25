package db

import (
	"context"
	"time"

	pgx "github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
)

// UserMemory represents a memory stored for a user
type UserMemory struct {
	ID     int64  `db:"id"`
	UserID string `db:"user_id"`
	Memory string `db:"memory"`
	// RoomID is the group room the memory was saved in, nil if it was saved in a DM
	RoomID    *string   `db:"room_id"`
	CreatedAt time.Time `db:"created_at"`
}

// MemoryView is where a user's memories are being used from. A DM sees all of them, a group room
// only the ones saved in it, so something said in private never surfaces in front of a group.
type MemoryView struct {
	RoomID string
	IsDM   bool
}

// SaveMemory saves a new memory for a user, scoped to where it was saved: a group room keeps it to
// itself and DMs, a DM keeps it to DMs
func SaveMemory(ctx context.Context, userID, memory string, savedIn MemoryView) error {
	var roomID *string
	if !savedIn.IsDM {
		roomID = &savedIn.RoomID
	}

	_, err := pool.Exec(ctx,
		"INSERT INTO user_memory (user_id, memory, room_id) VALUES ($1, $2, $3)",
		userID, memory, roomID)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("user_id", userID).
			Str("room_id", savedIn.RoomID).
			Bool("is_dm", savedIn.IsDM).
			Msg("Failed to save user memory")
		return err
	}
	return nil
}

// GetUserMemories returns the 100 most recent memories for a user, wherever they were saved
func GetUserMemories(ctx context.Context, userID string) ([]UserMemory, error) {
	rows, err := pool.Query(ctx,
		"SELECT id, user_id, memory, room_id, created_at FROM user_memory WHERE user_id = $1 ORDER BY created_at DESC LIMIT 100",
		userID)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("user_id", userID).Msg("Failed to query user memories")
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[UserMemory])
}

// GetMemoriesIn returns the 100 most recent memories for a user that are visible from view
func GetMemoriesIn(ctx context.Context, userID string, view MemoryView) ([]UserMemory, error) {
	rows, err := pool.Query(ctx,
		`SELECT id, user_id, memory, room_id, created_at FROM user_memory
		WHERE user_id = $1 AND ($2 OR room_id = $3)
		ORDER BY created_at DESC LIMIT 100`,
		userID, view.IsDM, view.RoomID)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("user_id", userID).
			Str("room_id", view.RoomID).
			Bool("is_dm", view.IsDM).
			Msg("Failed to query user memories")
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

// DeleteMemoryIn deletes a specific memory for a user if it is visible from view, reporting
// whether there was one to delete
func DeleteMemoryIn(ctx context.Context, userID string, memoryID int64, view MemoryView) (bool, error) {
	result, err := pool.Exec(ctx,
		"DELETE FROM user_memory WHERE id = $1 AND user_id = $2 AND ($3 OR room_id = $4)",
		memoryID, userID, view.IsDM, view.RoomID)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("user_id", userID).
			Int64("memory_id", memoryID).
			Str("room_id", view.RoomID).
			Bool("is_dm", view.IsDM).
			Msg("Failed to delete user memory")
		return false, err
	}
	return result.RowsAffected() > 0, nil
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

// DeleteAllMemoriesIn deletes all of a user's memories that are visible from view
func DeleteAllMemoriesIn(ctx context.Context, userID string, view MemoryView) (int64, error) {
	result, err := pool.Exec(ctx,
		"DELETE FROM user_memory WHERE user_id = $1 AND ($2 OR room_id = $3)",
		userID, view.IsDM, view.RoomID)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("user_id", userID).
			Str("room_id", view.RoomID).
			Bool("is_dm", view.IsDM).
			Msg("Failed to delete user memories")
		return 0, err
	}
	return result.RowsAffected(), nil
}

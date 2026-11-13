package db

import (
	"context"
	"time"

	pgx "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/rs/zerolog/log"
)

// RoomMemory is something the bot remembers about a group room, formed by thinking back over the
// conversations it had there
type RoomMemory struct {
	ID        int64     `db:"id"`
	RoomID    string    `db:"room_id"`
	Memory    string    `db:"memory"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

// RoomMemoryProgress is how far the bot has thought back over a room's chat history
type RoomMemoryProgress struct {
	RoomID string `db:"room_id"`
	// ReflectedUntil is the last chat_history row a pass covered
	ReflectedUntil int64 `db:"reflected_until"`
	// ReflectedAt is when a pass last completed, nil if none has
	ReflectedAt *time.Time `db:"reflected_at"`
	// FailedAttempts is how many passes have failed in a row since then
	FailedAttempts int       `db:"failed_attempts"`
	LastAttemptAt  time.Time `db:"last_attempt_at"`
}

// DueRoom is a room the bot has new conversations in to think back over
type DueRoom struct {
	RoomID string `db:"room_id"`
	// After is the last history row the bot has already thought back over, 0 if it never has, and
	// Until the room's latest one
	After int64 `db:"after"`
	Until int64 `db:"until"`
	// ReflectedAt and FailedAttempts are as in the room's progress, nil and 0 if it has none
	ReflectedAt    *time.Time `db:"reflected_at"`
	FailedAttempts int        `db:"failed_attempts"`
}

// The changes a pass can make to a room's memories
const (
	RoomMemoryAdd    = "add"
	RoomMemoryUpdate = "update"
	RoomMemoryDelete = "delete"
)

// RoomMemoryChange is one change a pass makes to a room's memories. MemoryID names the memory an
// update or delete changes, and Memory is the text an add or update gives.
type RoomMemoryChange struct {
	Action   string
	MemoryID int64
	Memory   string
}

// roomMemoryColumns are the columns of a RoomMemory
const roomMemoryColumns = "id, room_id, memory, created_at, updated_at"

// GetRoomMemories returns the bot's memories of a room, oldest first
func GetRoomMemories(ctx context.Context, roomID string) ([]RoomMemory, error) {
	rows, err := pool.Query(ctx,
		"SELECT "+roomMemoryColumns+" FROM room_memory WHERE room_id = $1 ORDER BY id",
		roomID)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Msg("Failed to query room memories")
		return nil, err
	}
	memories, err := pgx.CollectRows(rows, pgx.RowToStructByName[RoomMemory])
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Msg("Failed to collect room memories")
		return nil, err
	}
	return memories, nil
}

// GetAllRoomMemories returns the bot's memories of every room, each room's oldest first
func GetAllRoomMemories(ctx context.Context) ([]RoomMemory, error) {
	rows, err := pool.Query(ctx, "SELECT "+roomMemoryColumns+" FROM room_memory ORDER BY room_id, id")
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to query all room memories")
		return nil, err
	}
	memories, err := pgx.CollectRows(rows, pgx.RowToStructByName[RoomMemory])
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to collect all room memories")
		return nil, err
	}
	return memories, nil
}

// GetAllRoomMemoryProgress returns how far the bot has thought back over each room it has tried to
func GetAllRoomMemoryProgress(ctx context.Context) ([]RoomMemoryProgress, error) {
	rows, err := pool.Query(ctx,
		`SELECT room_id, reflected_until, reflected_at, failed_attempts, last_attempt_at
		FROM room_memory_progress`)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to query room memory progress")
		return nil, err
	}
	progress, err := pgx.CollectRows(rows, pgx.RowToStructByName[RoomMemoryProgress])
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to collect room memory progress")
		return nil, err
	}
	return progress, nil
}

// RoomsDueForReflection returns the rooms whose chat history has rows the bot hasn't thought back
// over, and that have been quiet since before quietSince: their latest row is older, and so is their
// latest attempt at a pass, so a pass that failed waits as long before it is tried again. The rooms
// that have been quiet the longest come first.
func RoomsDueForReflection(ctx context.Context, quietSince time.Time) ([]DueRoom, error) {
	rows, err := pool.Query(ctx,
		`SELECT h.room_id,
			COALESCE(p.reflected_until, 0) AS after,
			MAX(h.id)::bigint AS until,
			p.reflected_at,
			COALESCE(p.failed_attempts, 0) AS failed_attempts
		FROM chat_history h
		LEFT JOIN room_memory_progress p ON p.room_id = h.room_id
		WHERE h.id > COALESCE(p.reflected_until, 0)
		GROUP BY h.room_id, p.reflected_until, p.reflected_at, p.failed_attempts, p.last_attempt_at
		HAVING MAX(h.timestamp) < $1 AND (p.last_attempt_at IS NULL OR p.last_attempt_at < $1)
		ORDER BY MAX(h.timestamp)`,
		quietSince)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Time("quiet_since", quietSince).Msg("Failed to query rooms due for reflection")
		return nil, err
	}
	rooms, err := pgx.CollectRows(rows, pgx.RowToStructByName[DueRoom])
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Time("quiet_since", quietSince).Msg("Failed to collect rooms due for reflection")
		return nil, err
	}
	return rooms, nil
}

// GetConversationsSince returns the people's messages and the bot's answers in a room's chat history
// after the row after, up to and including the row until, across all of its timelines and in the
// order they were written. The user rows come with the messages they refer to, but not with their
// images. Tool calls and their results are left out.
func GetConversationsSince(ctx context.Context, roomID string, after, until int64) ([]ChatMessage, error) {
	rows, err := pool.Query(ctx,
		`SELECT id, room_id, user_id, message, role, timestamp, message_type, tool_call_id, tool_name, expiry,
			turn_event_id, thread_root_id, sender_name, sent_at, mentions, unseen_before, has_image,
			answer_event_id
		FROM chat_history
		WHERE room_id = $1 AND id > $2 AND id <= $3 AND message_type = 'text'
		ORDER BY timestamp, id`,
		roomID, after, until)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", roomID).
			Int64("after", after).
			Int64("until", until).
			Msg("Failed to get conversations to reflect on")
		return nil, err
	}
	messages, err := pgx.CollectRows(rows, pgx.RowToStructByName[ChatMessage])
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Msg("Failed to collect conversations to reflect on")
		return nil, err
	}
	if err := loadReferences(ctx, roomID, messages); err != nil {
		return nil, err
	}
	return messages, nil
}

// ApplyRoomMemoryChanges makes the changes a pass decided on to a room's memories, in order, and
// records the progress the pass made, in one transaction. An update or delete of a memory that is no
// longer there, deleted on the dashboard while the pass ran, changes nothing. Returns the changes that
// took effect.
func ApplyRoomMemoryChanges(ctx context.Context, roomID string, changes []RoomMemoryChange, progress RoomMemoryProgress) ([]RoomMemoryChange, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Msg("Failed to begin transaction for room memory changes")
		return nil, err
	}
	defer tx.Rollback(ctx)

	var applied []RoomMemoryChange
	for _, change := range changes {
		var tag pgconn.CommandTag
		switch change.Action {
		case RoomMemoryAdd:
			tag, err = tx.Exec(ctx,
				"INSERT INTO room_memory (room_id, memory) VALUES ($1, $2)",
				roomID, change.Memory)
		case RoomMemoryUpdate:
			tag, err = tx.Exec(ctx,
				"UPDATE room_memory SET memory = $3, updated_at = NOW() WHERE id = $1 AND room_id = $2",
				change.MemoryID, roomID, change.Memory)
		case RoomMemoryDelete:
			tag, err = tx.Exec(ctx,
				"DELETE FROM room_memory WHERE id = $1 AND room_id = $2",
				change.MemoryID, roomID)
		}
		if err != nil {
			log.Error().Ctx(ctx).Err(err).
				Str("room_id", roomID).
				Str("action", change.Action).
				Int64("memory_id", change.MemoryID).
				Msg("Failed to change a room memory")
			return nil, err
		}
		if tag.RowsAffected() > 0 {
			applied = append(applied, change)
		}
	}

	if err := saveRoomMemoryProgress(ctx, tx, progress); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Msg("Failed to commit room memory changes")
		return nil, err
	}
	return applied, nil
}

// SaveRoomMemoryProgress records how far the bot has thought back over a room, for a pass that
// changed no memories: one that failed, or had nothing to think back over
func SaveRoomMemoryProgress(ctx context.Context, progress RoomMemoryProgress) error {
	return saveRoomMemoryProgress(ctx, pool, progress)
}

// saveRoomMemoryProgress writes a room's progress with the given executor, so it can run either on the
// pool or inside a transaction
func saveRoomMemoryProgress(ctx context.Context, executor chatMessageExecutor, progress RoomMemoryProgress) error {
	_, err := executor.Exec(ctx,
		`INSERT INTO room_memory_progress (room_id, reflected_until, reflected_at, failed_attempts, last_attempt_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (room_id) DO UPDATE SET
			reflected_until = EXCLUDED.reflected_until,
			reflected_at = EXCLUDED.reflected_at,
			failed_attempts = EXCLUDED.failed_attempts,
			last_attempt_at = EXCLUDED.last_attempt_at`,
		progress.RoomID, progress.ReflectedUntil, progress.ReflectedAt, progress.FailedAttempts, progress.LastAttemptAt)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", progress.RoomID).
			Int64("reflected_until", progress.ReflectedUntil).
			Int("failed_attempts", progress.FailedAttempts).
			Msg("Failed to save room memory progress")
		return err
	}
	return nil
}

// DeleteRoomMemory deletes one of the bot's memories of a room, reporting whether there was one to
// delete
func DeleteRoomMemory(ctx context.Context, memoryID int64) (bool, error) {
	tag, err := pool.Exec(ctx, "DELETE FROM room_memory WHERE id = $1", memoryID)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Int64("memory_id", memoryID).Msg("Failed to delete room memory")
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// DeleteRoomMemories deletes all of the bot's memories of a room, returning how many there were
func DeleteRoomMemories(ctx context.Context, roomID string) (int64, error) {
	tag, err := pool.Exec(ctx, "DELETE FROM room_memory WHERE room_id = $1", roomID)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Msg("Failed to delete room memories")
		return 0, err
	}
	return tag.RowsAffected(), nil
}

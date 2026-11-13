package db

import (
	"context"
	"errors"
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
	// CreatedTraceID is the trace of the pass that formed the memory, and UpdatedTraceID of the one that
	// last revised it, the same if none has. Nil when it wasn't recorded.
	CreatedTraceID *string `db:"created_trace_id"`
	UpdatedTraceID *string `db:"updated_trace_id"`
}

// RoomMemoryProgress is how far the bot has thought back over a room's chat history, and how its latest
// pass went
type RoomMemoryProgress struct {
	RoomID string `db:"room_id"`
	// ReflectedUntil is the last chat_history row a pass covered
	ReflectedUntil int64 `db:"reflected_until"`
	// ReflectedAt is when a pass last completed, nil if none has
	ReflectedAt *time.Time `db:"reflected_at"`
	// FailedAttempts is how many passes have failed in a row since then
	FailedAttempts int       `db:"failed_attempts"`
	LastAttemptAt  time.Time `db:"last_attempt_at"`
	// LastOutcome is how the latest pass went, such as unchanged or request_failed, and LastTraceID its
	// trace. Nil when not recorded.
	LastOutcome *string `db:"last_outcome"`
	LastTraceID *string `db:"last_trace_id"`
}

// RoomMemoryState is where the bot stands with a room: how far it has thought back over the room's
// chat history, what is new since, and how its latest pass went
type RoomMemoryState struct {
	RoomID string `db:"room_id"`
	// After is the last history row the bot has thought back over, 0 if it never has, and Until the
	// room's latest one. There is something new to think back over when Until is past After.
	After int64 `db:"after"`
	Until int64 `db:"until"`
	// LatestAt is when the latest row the bot hasn't thought back over was written, nil if there is none
	LatestAt *time.Time `db:"latest_at"`
	// The rest is as in the room's progress, nil and 0 when it has none
	ReflectedAt    *time.Time `db:"reflected_at"`
	FailedAttempts int        `db:"failed_attempts"`
	LastAttemptAt  *time.Time `db:"last_attempt_at"`
	LastOutcome    *string    `db:"last_outcome"`
	LastTraceID    *string    `db:"last_trace_id"`
}

// HasNewHistory reports whether the room's chat history has rows the bot hasn't thought back over
func (s RoomMemoryState) HasNewHistory() bool {
	return s.Until > s.After
}

// The changes a pass can make to a room's memories
const (
	RoomMemoryAdd    = "add"
	RoomMemoryUpdate = "update"
	RoomMemoryDelete = "delete"
)

// RoomMemoryChange is one change a pass makes to a room's memories. MemoryID names the memory an
// update or delete changes, and the one an add created once it's applied. Memory is the text an add
// or update gives.
type RoomMemoryChange struct {
	Action   string
	MemoryID int64
	Memory   string
}

// roomMemoryColumns are the columns of a RoomMemory
const roomMemoryColumns = "id, room_id, memory, created_at, updated_at, created_trace_id, updated_trace_id"

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

// CountRoomMemories returns how many memories the bot keeps of all rooms together
func CountRoomMemories(ctx context.Context) (int, error) {
	var count int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM room_memory").Scan(&count); err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to count room memories")
		return 0, err
	}
	return count, nil
}

// roomMemoryStateQuery reads the state of every room the bot has memories of, has tried to think back
// over, or has new chat history in. pending is each room's history the bot hasn't thought back over.
const roomMemoryStateQuery = `WITH pending AS (
		SELECT h.room_id, MAX(h.id)::bigint AS until, MAX(h.timestamp) AS latest_at
		FROM chat_history h
		LEFT JOIN room_memory_progress p ON p.room_id = h.room_id
		WHERE h.id > COALESCE(p.reflected_until, 0)
		GROUP BY h.room_id
	), rooms AS (
		SELECT room_id FROM pending
		UNION SELECT room_id FROM room_memory_progress
		UNION SELECT room_id FROM room_memory
	)
	SELECT r.room_id,
		COALESCE(p.reflected_until, 0) AS after,
		COALESCE(pe.until, p.reflected_until, 0) AS until,
		pe.latest_at,
		p.reflected_at,
		COALESCE(p.failed_attempts, 0) AS failed_attempts,
		p.last_attempt_at,
		p.last_outcome,
		p.last_trace_id
	FROM rooms r
	LEFT JOIN room_memory_progress p ON p.room_id = r.room_id
	LEFT JOIN pending pe ON pe.room_id = r.room_id`

// queryRoomMemoryStates runs roomMemoryStateQuery with a condition and order of its own
func queryRoomMemoryStates(ctx context.Context, rest string, args ...any) ([]RoomMemoryState, error) {
	rows, err := pool.Query(ctx, roomMemoryStateQuery+" "+rest, args...)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to query room memory states")
		return nil, err
	}
	states, err := pgx.CollectRows(rows, pgx.RowToStructByName[RoomMemoryState])
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to collect room memory states")
		return nil, err
	}
	return states, nil
}

// RoomsWithNewHistory returns the state of each room whose chat history has rows the bot hasn't thought
// back over, the room that has been quiet the longest first. Whether a room is due a pass is for the
// caller to decide from its state.
func RoomsWithNewHistory(ctx context.Context) ([]RoomMemoryState, error) {
	return queryRoomMemoryStates(ctx, "WHERE pe.room_id IS NOT NULL ORDER BY pe.latest_at")
}

// GetAllRoomMemoryStates returns the state of every room the bot has memories of, has tried to think
// back over, or has new chat history in
func GetAllRoomMemoryStates(ctx context.Context) ([]RoomMemoryState, error) {
	return queryRoomMemoryStates(ctx, "ORDER BY r.room_id")
}

// GetRoomMemoryState returns the state of one room. A room the bot has never had anything to do with
// has nothing new to think back over.
func GetRoomMemoryState(ctx context.Context, roomID string) (RoomMemoryState, error) {
	states, err := queryRoomMemoryStates(ctx, "WHERE r.room_id = $1", roomID)
	if err != nil {
		return RoomMemoryState{}, err
	}
	if len(states) == 0 {
		return RoomMemoryState{RoomID: roomID}, nil
	}
	return states[0], nil
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
// records the progress the pass made, in one transaction. Each memory it adds or updates records
// traceID, the pass's trace, nil if it has none. An update or delete of a memory that is no longer
// there, deleted on the dashboard while the pass ran, changes nothing. Returns the changes that took
// effect, an add with the ID of the memory it created.
func ApplyRoomMemoryChanges(ctx context.Context, roomID string, changes []RoomMemoryChange, progress RoomMemoryProgress, traceID *string) ([]RoomMemoryChange, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Msg("Failed to begin transaction for room memory changes")
		return nil, err
	}
	defer tx.Rollback(ctx)

	var applied []RoomMemoryChange
	for _, change := range changes {
		var affected int64
		switch change.Action {
		case RoomMemoryAdd:
			err = tx.QueryRow(ctx,
				`INSERT INTO room_memory (room_id, memory, created_trace_id, updated_trace_id)
				VALUES ($1, $2, $3, $3) RETURNING id`,
				roomID, change.Memory, traceID).Scan(&change.MemoryID)
			affected = 1
		case RoomMemoryUpdate:
			var tag pgconn.CommandTag
			tag, err = tx.Exec(ctx,
				`UPDATE room_memory SET memory = $3, updated_at = NOW(), updated_trace_id = $4
				WHERE id = $1 AND room_id = $2`,
				change.MemoryID, roomID, change.Memory, traceID)
			affected = tag.RowsAffected()
		case RoomMemoryDelete:
			var tag pgconn.CommandTag
			tag, err = tx.Exec(ctx,
				"DELETE FROM room_memory WHERE id = $1 AND room_id = $2",
				change.MemoryID, roomID)
			affected = tag.RowsAffected()
		}
		if err != nil {
			log.Error().Ctx(ctx).Err(err).
				Str("room_id", roomID).
				Str("action", change.Action).
				Int64("memory_id", change.MemoryID).
				Msg("Failed to change a room memory")
			return nil, err
		}
		if affected > 0 {
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
		`INSERT INTO room_memory_progress
			(room_id, reflected_until, reflected_at, failed_attempts, last_attempt_at, last_outcome, last_trace_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (room_id) DO UPDATE SET
			reflected_until = EXCLUDED.reflected_until,
			reflected_at = EXCLUDED.reflected_at,
			failed_attempts = EXCLUDED.failed_attempts,
			last_attempt_at = EXCLUDED.last_attempt_at,
			last_outcome = EXCLUDED.last_outcome,
			last_trace_id = EXCLUDED.last_trace_id`,
		progress.RoomID, progress.ReflectedUntil, progress.ReflectedAt, progress.FailedAttempts, progress.LastAttemptAt,
		progress.LastOutcome, progress.LastTraceID)
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

// DeleteRoomMemory deletes one of the bot's memories of a room, returning it, or nil if there was none
// to delete
func DeleteRoomMemory(ctx context.Context, memoryID int64) (*RoomMemory, error) {
	rows, err := pool.Query(ctx, "DELETE FROM room_memory WHERE id = $1 RETURNING "+roomMemoryColumns, memoryID)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Int64("memory_id", memoryID).Msg("Failed to delete room memory")
		return nil, err
	}
	memory, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[RoomMemory])
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Int64("memory_id", memoryID).Msg("Failed to delete room memory")
		return nil, err
	}
	return &memory, nil
}

// DeleteRoomMemories deletes all of the bot's memories of a room, returning them
func DeleteRoomMemories(ctx context.Context, roomID string) ([]RoomMemory, error) {
	rows, err := pool.Query(ctx, "DELETE FROM room_memory WHERE room_id = $1 RETURNING "+roomMemoryColumns, roomID)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Msg("Failed to delete room memories")
		return nil, err
	}
	memories, err := pgx.CollectRows(rows, pgx.RowToStructByName[RoomMemory])
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Msg("Failed to delete room memories")
		return nil, err
	}
	return memories, nil
}

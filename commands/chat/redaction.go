package chat

import (
	"context"
	"sync"
	"time"

	"github.com/Scrin/siikabot/db"
	"github.com/rs/zerolog/log"
)

// A message deleted from the room leaves the bot's chat history too. That covers a message that
// triggered a turn, the bot's answer to one, and a message a turn quoted. A turn still running, or
// waiting for the room, when its trigger is deleted stops: it stores nothing more and posts no
// answer.

// redactionMemory is how long a redaction is remembered for turns that haven't finished yet. It has
// to outlast any turn, including its wait for the room lock.
const redactionMemory = time.Hour

var redactions = struct {
	sync.Mutex
	at map[string]time.Time
}{at: make(map[string]time.Time)}

// markRedacted remembers that an event was redacted, for turns that haven't finished yet
func markRedacted(eventID string) {
	redactions.Lock()
	defer redactions.Unlock()

	now := time.Now()
	for id, at := range redactions.at {
		if now.Sub(at) > redactionMemory {
			delete(redactions.at, id)
		}
	}
	redactions.at[eventID] = now
}

// isRedacted reports whether an event was redacted recently
func isRedacted(eventID string) bool {
	redactions.Lock()
	defer redactions.Unlock()

	_, ok := redactions.at[eventID]
	return ok
}

// ForgetEvent handles the redaction of an event in a room: whatever the chat history holds of it
// goes, and a turn it triggered that is still running stops
func ForgetEvent(ctx context.Context, roomID, eventID string) {
	markRedacted(eventID)

	deleted, blanked, err := db.ForgetChatEvent(ctx, roomID, eventID)
	if err != nil {
		// Already logged
		return
	}
	if deleted > 0 || blanked > 0 {
		log.Info().Ctx(ctx).
			Str("room_id", roomID).
			Str("event_id", eventID).
			Int64("deleted_rows", deleted).
			Int64("blanked_quotes", blanked).
			Msg("Removed a redacted event from the chat history")
	}
}

// forgotten reports whether a turn's trigger has been redacted. If it has, whatever the turn
// already stored is removed as well, since some of it may have been written after the redaction
// was handled.
func forgotten(ctx context.Context, turn db.Turn) bool {
	if !isRedacted(turn.EventID) {
		return false
	}

	persistCtx, cancel := persistContext(ctx)
	defer cancel()
	if _, _, err := db.ForgetChatEvent(persistCtx, turn.RoomID, turn.EventID); err != nil {
		// Already logged
		_ = err
	}
	return true
}

// persist runs one of a turn's history writes, on a context that outlives a cancelled turn, unless
// the turn's trigger has been redacted. If the redaction arrives while the write is in progress,
// the check after it removes what was written.
func persist(ctx context.Context, turn db.Turn, record string, write func(context.Context) error) {
	if forgotten(ctx, turn) {
		return
	}

	persistCtx, cancel := persistContext(ctx)
	defer cancel()
	if err := write(persistCtx); err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", turn.RoomID).
			Str("turn_event_id", turn.EventID).
			Str("record", record).
			Msg("Failed to save chat history")
		// Continue even if saving fails
	}

	forgotten(ctx, turn)
}

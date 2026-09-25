package chat

import (
	"context"
	"slices"

	"github.com/Scrin/siikabot/db"
	"github.com/Scrin/siikabot/matrix"
)

// Each thread has a context window of its own, apart from the main timeline and from other threads.
// A thread started from a stored turn, on the bot's answer or on the question it answered, begins
// with that turn: the question, the tool calls and their results, the answer, and then the thread's
// own messages. The turn stays in the main timeline and is only referred to, so a redaction or a
// reset there removes it from the thread as well.

// threadSeed returns the stored turn a thread was started from, whose rows open the thread's
// context. It returns an empty string in the main timeline, for a thread started on any other
// message, and for a thread whose opening turn the person asking couldn't see: a thread started by
// hand on a turn from before they joined would otherwise show it to them, which is the same check a
// reply to it gets (see matrix.VisibleTo).
func threadSeed(ctx context.Context, timeline db.Timeline, requester string) string {
	if timeline.ThreadRootID == "" {
		return ""
	}
	seed, found, err := db.GetThreadSeed(ctx, timeline.RoomID, timeline.ThreadRootID)
	if err != nil || !found {
		// A failure is already logged, and the thread goes ahead on its own messages
		return ""
	}
	if !matrix.VisibleTo(ctx, timeline.RoomID, requester, seed.SentAt) {
		return ""
	}
	return seed.TurnEventID
}

// windowHasTurn reports whether a context window still holds the rows of a turn. The turn a thread
// was started from is the first thing to go when the thread's window outgrows its budget.
func windowHasTurn(history []db.ChatMessage, turnEventID string) bool {
	return turnEventID != "" && slices.ContainsFunc(history, func(row db.ChatMessage) bool {
		return row.TurnEventID == turnEventID
	})
}

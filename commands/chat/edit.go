package chat

import (
	"context"
	"sync"
	"time"

	"github.com/Scrin/siikabot/db"
	"github.com/rs/zerolog/log"
)

// An edit is remembered, as a redaction is (see redaction.go), for the turns that haven't stored what
// they hold of the message yet. A turn takes its question, and fetches the messages it quotes, when
// the message is routed; one waiting for the room uses their latest edits in its prompt, and one that
// has stored them before an edit was followed follows it after.

// edited is a message's latest edit by one sender
type edited struct {
	// text is the message as its turn shows it, and quoted as a quote shows it
	text, quoted string
	at           time.Time
}

// edits are the latest edits of each message, by sender, since only the sender's own edits count.
// The lock is held while an edit is stored, so that one followed again after a turn's write never
// lands after a later one.
var edits = struct {
	sync.Mutex
	latest map[string]map[string]edited
}{latest: make(map[string]map[string]edited)}

// FollowEdit brings the chat history in line with an edit of a message: a turn the message started,
// and any quote of it, now read as edited. An edit is often a correction, or a retraction, and the
// history keeps the words its sender settled on, just as it drops a message they delete. Only the
// sender's own edits count.
//
// text is the message as its turn shows it, without a leading address to the bot. quoted is all of
// what the sender wrote, as a quote shows it.
func FollowEdit(ctx context.Context, roomID, eventID, sender, text, quoted string) {
	quoted = capQuote(quoted)

	edits.Lock()
	defer edits.Unlock()

	now := time.Now()
	for id, bySender := range edits.latest {
		for who, edit := range bySender {
			if now.Sub(edit.at) > redactionMemory {
				delete(bySender, who)
			}
		}
		if len(bySender) == 0 {
			delete(edits.latest, id)
		}
	}
	if edits.latest[eventID] == nil {
		edits.latest[eventID] = make(map[string]edited)
	}
	edits.latest[eventID][sender] = edited{text: text, quoted: quoted, at: now}

	turns, references, err := db.FollowChatEdit(ctx, roomID, eventID, sender, text, quoted)
	if err != nil {
		// Already logged
		return
	}
	if turns > 0 || references > 0 {
		log.Info().Ctx(ctx).
			Str("room_id", roomID).
			Str("event_id", eventID).
			Int64("changed_turns", turns).
			Int64("changed_references", references).
			Msg("Followed an edit in the chat history")
	}
}

// latestEdit returns a message's latest edit by its sender, if it was edited recently
func latestEdit(eventID, sender string) (edited, bool) {
	edits.Lock()
	defer edits.Unlock()

	edit, ok := edits.latest[eventID][sender]
	return edit, ok
}

// followEdits brings what a turn has stored of messages in line with their latest edits, made while
// the turn ran and followed before it stored them. The history checks each edit's sender.
func followEdits(ctx context.Context, turn db.Turn, record string, eventIDs ...string) {
	edits.Lock()
	defer edits.Unlock()

	for _, eventID := range eventIDs {
		for sender, edit := range edits.latest[eventID] {
			turns, references, err := db.FollowChatEdit(ctx, turn.RoomID, eventID, sender, edit.text, edit.quoted)
			if err != nil {
				// Already logged
				continue
			}
			if turns > 0 || references > 0 {
				log.Debug().Ctx(ctx).
					Str("room_id", turn.RoomID).
					Str("turn_event_id", turn.EventID).
					Str("event_id", eventID).
					Str("record", record).
					Msg("Followed an edit made while a turn ran")
			}
		}
	}
}

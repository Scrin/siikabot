package chat

import (
	"context"

	"github.com/Scrin/siikabot/db"
	"github.com/rs/zerolog/log"
)

// FollowEdit brings the chat history in line with an edit of a message: a turn the message started,
// and any quote of it, now read as edited. An edit is often a correction, or a retraction, and the
// history keeps the words its sender settled on, just as it drops a message they delete. Only the
// sender's own edits count.
//
// text is the message as its turn shows it, without a leading address to the bot. quoted is all of
// what the sender wrote, as a quote shows it.
func FollowEdit(ctx context.Context, roomID, eventID, sender, text, quoted string) {
	turns, references, err := db.FollowChatEdit(ctx, roomID, eventID, sender, text, capQuote(quoted))
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

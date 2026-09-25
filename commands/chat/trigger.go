package chat

import (
	"time"

	"github.com/Scrin/siikabot/matrix"
)

// Trigger is a message that addressed the bot, with everything a chat turn needs to know about it.
//
// It is put together once, where the message is routed, so the turn never goes back to the room
// for more. In particular the turn never fetches events itself: the one event the message refers
// to arrives here already fetched, and nothing else from the room is available to it.
type Trigger struct {
	RoomID    string
	Sender    string
	EventID   string
	Timestamp time.Time

	// Body is what the sender asked: without a reply's quote of the message it replies to, and
	// without the bot's name if the message opened with it
	Body string
	// FormattedBody is the HTML body as sent, without a reply's quote
	FormattedBody string
	// Mentions are the users listed in the message's m.mentions
	Mentions []string
	// ThreadRootID is the root of the thread the message was sent in, empty in the main timeline
	ThreadRootID string

	// ReplyToEventID is the event the message explicitly refers to: the message it replies to, or
	// the root of the thread it starts. Empty if it refers to none.
	ReplyToEventID string
	// ReplyTo is that event, or nil if it couldn't be fetched
	ReplyTo *matrix.Message
}

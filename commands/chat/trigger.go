package chat

import (
	"time"

	"github.com/Scrin/siikabot/matrix"
)

// Trigger is a message that addressed the bot, with everything a chat turn needs to know about it.
//
// It is put together once, where the message is routed, so the turn never goes back to the room
// for more. In particular the turn never fetches events itself: the events the message refers to
// arrive here already fetched, and nothing else from the room is available to it.
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
	// Mentions are the users listed in the message's m.mentions. Nil means the message has no
	// m.mentions at all, as from an older client, which leaves its pills as the only record.
	Mentions []string
	// ThreadRootID is the root of the thread the message was sent in, empty in the main timeline
	ThreadRootID string
	// UnseenBefore is how many room messages the bot did not see between the previous message
	// addressed to it and this one
	UnseenBefore int

	// ReplyToEventID is the event the message explicitly replies to: the message it replies to, or
	// the root of the thread it starts. Empty if it replies to none.
	ReplyToEventID string
	// ReplyTo is that event, or nil if it couldn't be fetched or its sender couldn't see it
	ReplyTo *matrix.Message
	// Links are the messages in the room that the message links to, in the order it links to them
	Links []Link

	// Image is the message itself when it is an image, with Body as its caption. Nil for text.
	Image *matrix.Message

	// ByReplyOnly says the message addressed the bot only by replying to one of its messages, or
	// by starting a thread on one: it neither mentions the bot nor opens with its name. Such a
	// message may need no answer at all, like a thanks, and the model may then stay silent.
	ByReplyOnly bool
}

// Link is a message in the room that a message addressed to the bot links to
type Link struct {
	EventID string
	// Message is the linked message, or nil if it couldn't be fetched or its sender couldn't see it
	Message *matrix.Message
}

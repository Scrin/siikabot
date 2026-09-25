package bot

import (
	"context"
	"html"
	"regexp"
	"strings"

	"github.com/Scrin/siikabot/commands/chat"
	"github.com/Scrin/siikabot/matrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// A message may explicitly refer to other events: the one it replies to, and the ones in the room it
// links to. Those events are the only room content beyond the message itself that the chat model
// may see (the chat privacy invariant in CLAUDE.md), so which events they are gets decided here,
// once, and nowhere else.

// replyTarget returns the event a message explicitly refers to, or an empty id if it refers to
// none: the message it replies to, or the root of the thread it starts.
//
// A thread message carries a reply relation too, but a fallback one. It points at whatever the
// sender's client took to be the latest message in the thread, for the benefit of clients that
// don't show threads. Nobody chose that message, so it doesn't count.
//
// The first message of a thread is the exception. Starting a thread on a message is as deliberate
// as replying to it, so that message refers to the root. Its fallback points at the root itself,
// or is missing if the client sends none; a fallback pointing anywhere else means the thread
// already had replies. isFirstThreadReply is still asked to confirm, since a client could point
// every fallback at the root.
func replyTarget(rel *event.RelatesTo, isFirstThreadReply func(root id.EventID) bool) id.EventID {
	if target := rel.GetNonFallbackReplyTo(); target != "" {
		return target
	}

	root := rel.GetThreadParent()
	if root == "" {
		return ""
	}
	if fallback := rel.GetReplyTo(); fallback != "" && fallback != root {
		return ""
	}
	if !isFirstThreadReply(root) {
		return ""
	}
	return root
}

// ownText returns what the sender of a message wrote themselves, without the quote of an earlier
// message that a reply may open with. Only a reply carries such a quote, so a message that merely
// opens with a quote of its own is left alone.
func ownText(body, formattedBody string, rel *event.RelatesTo) (string, string) {
	if rel.GetReplyTo() == "" {
		return body, formattedBody
	}
	return strings.TrimSpace(matrix.StripReplyFallback(body)), matrix.StripMxReply(formattedBody)
}

// fetchReferenced fetches a message another one refers to, as far as the sender of that one may
// see it. It returns nil for a message that can't be fetched, and for one sent before the sender's
// membership began in a room that hides such history from them (see matrix.VisibleTo). Either way
// the chat turn learns only that there was a message it couldn't read.
func fetchReferenced(ctx context.Context, roomID, sender, eventID string) *matrix.Message {
	msg, err := matrix.FetchMessage(ctx, roomID, eventID)
	if err != nil {
		// Already logged
		return nil
	}
	if !matrix.VisibleTo(ctx, roomID, sender, msg.Timestamp) {
		return nil
	}
	return msg
}

// maxLinks caps how many of the messages a message links to its chat turn gets. A message that
// links to more is pasting a conversation rather than pointing at something in it.
const maxLinks = 3

// eventLink matches a link that can point at an event: a matrix.to link or a matrix: URI
var eventLink = regexp.MustCompile(`https://matrix\.to/#/[^\s"'<>]+|matrix:(?:roomid|r|room)/[^\s"'<>]+`)

// linkedMessages fetches the messages in the room that a message links to, in order and at most
// maxLinks of them, leaving out the ones excluded: the message itself, and the one it replies to,
// which the turn has already. The message's own text and formatted text are both searched, since a
// client may write a link into either. Like the message a reply refers to, a linked message is left
// unread when its sender couldn't see it (see fetchReferenced).
func linkedMessages(ctx context.Context, roomID, sender, body, formattedBody string, exclude ...string) []chat.Link {
	var links []chat.Link
	for _, eventID := range linkedEventIDs(ctx, roomID, body, formattedBody, exclude...) {
		links = append(links, chat.Link{EventID: eventID, Message: fetchReferenced(ctx, roomID, sender, eventID)})
	}
	return links
}

// linkedEventIDs returns the events in the room that a message links to, as linkedMessages uses
// them
func linkedEventIDs(ctx context.Context, roomID, body, formattedBody string, exclude ...string) []string {
	seen := make(map[string]bool, len(exclude))
	for _, eventID := range exclude {
		seen[eventID] = true
	}

	var eventIDs []string
	for _, candidate := range append(eventLink.FindAllString(body, -1), eventLink.FindAllString(formattedBody, -1)...) {
		eventID, ok := linkedEvent(ctx, roomID, candidate)
		if !ok || seen[eventID] {
			continue
		}
		seen[eventID] = true
		eventIDs = append(eventIDs, eventID)
		if len(eventIDs) == maxLinks {
			break
		}
	}
	return eventIDs
}

// linkedEvent returns the event a link points at, if it is an event in the room: linked by the
// room's ID, or by an alias of the room
func linkedEvent(ctx context.Context, roomID, link string) (string, bool) {
	// Punctuation after a link ends the sentence it is in rather than the link, and a link taken
	// from HTML has its ampersands escaped
	link = html.UnescapeString(strings.TrimRight(link, ".,;:!?)"))
	uri, err := id.ParseMatrixURIOrMatrixToURL(link)
	if err != nil || uri.EventID() == "" {
		return "", false
	}

	switch {
	case uri.RoomID() != "":
		if uri.RoomID().String() != roomID {
			return "", false
		}
	case uri.RoomAlias() != "":
		resolved, err := matrix.ResolveRoomAlias(ctx, uri.RoomAlias().String())
		if err != nil || resolved != roomID {
			// A failure is already logged
			return "", false
		}
	default:
		return "", false
	}
	return uri.EventID().String(), true
}

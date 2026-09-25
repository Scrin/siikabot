package bot

import (
	"strings"

	"github.com/Scrin/siikabot/matrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// A message may explicitly refer to one other event. That event is the only room content beyond
// the message itself that the chat model may see (the chat privacy invariant in CLAUDE.md), so
// which event it is gets decided here, once, and nowhere else.

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

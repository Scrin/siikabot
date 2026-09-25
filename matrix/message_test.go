package matrix

import (
	"context"
	"strings"
	"testing"
	"time"

	"maunium.net/go/mautrix/event"
)

const testRoomID = "!room:example.com"

// messageEvent builds a fetched m.room.message event from Bob with the given content
func messageEvent(content map[string]any) *event.Event {
	return &event.Event{
		ID:        "$bob",
		Sender:    "@bob:example.com",
		Type:      event.EventMessage,
		Timestamp: 1758800000000,
		Content:   event.Content{Raw: content},
	}
}

// Bob's message is itself a reply to Carol's, and his client quoted her message in it. Whoever
// referred to Bob's message did not refer to Carol's, so her words must not come along.
func TestMessageFromEventStripsNestedFallback(t *testing.T) {
	msg := messageFromEvent(testRoomID, messageEvent(map[string]any{
		"msgtype":        "m.text",
		"body":           "> <@carol:example.com> Carol's original words\n\nI agree with this",
		"format":         "org.matrix.custom.html",
		"formatted_body": "<mx-reply><blockquote>Carol's original words</blockquote></mx-reply>I agree with <b>this</b>",
		"m.relates_to":   map[string]any{"m.in_reply_to": map[string]any{"event_id": "$carol"}},
	}))

	if msg.Body != "I agree with this" {
		t.Errorf("Body = %q, want only Bob's own words", msg.Body)
	}
	if strings.Contains(msg.FormattedBody, "Carol") {
		t.Errorf("FormattedBody = %q, still carries the quoted message", msg.FormattedBody)
	}
	if msg.FormattedBody != "I agree with <b>this</b>" {
		t.Errorf("FormattedBody = %q, want Bob's own formatted words", msg.FormattedBody)
	}
}

// Only a reply carries a fallback. A message that merely opens with a quote of its own keeps it.
func TestMessageFromEventKeepsAQuoteThatIsNotAReply(t *testing.T) {
	body := "> a line I am quoting myself\n\nthoughts?"
	msg := messageFromEvent(testRoomID, messageEvent(map[string]any{
		"msgtype": "m.text",
		"body":    body,
	}))

	if msg.Body != body {
		t.Errorf("Body = %q, want it unchanged", msg.Body)
	}
}

func TestMessageFromEventReadsTheEnvelope(t *testing.T) {
	msg := messageFromEvent(testRoomID, messageEvent(map[string]any{"msgtype": "m.text", "body": "hi"}))

	if msg.RoomID != testRoomID || msg.EventID != "$bob" || msg.Sender != "@bob:example.com" {
		t.Errorf("envelope = %q %q %q, want the room, event and sender", msg.RoomID, msg.EventID, msg.Sender)
	}
	if !msg.Timestamp.Equal(time.UnixMilli(1758800000000)) {
		t.Errorf("Timestamp = %v, want the event's origin_server_ts", msg.Timestamp)
	}
	if msg.MsgType != "m.text" || msg.Body != "hi" {
		t.Errorf("MsgType = %q, Body = %q", msg.MsgType, msg.Body)
	}
}

// Content that could not be decrypted is still encrypted: the sender is known, the content is not
func TestMessageFromEventThatCouldNotBeDecrypted(t *testing.T) {
	evt := &event.Event{
		ID:      "$secret",
		Sender:  "@bob:example.com",
		Type:    event.EventEncrypted,
		Content: event.Content{Raw: map[string]any{"algorithm": "m.megolm.v1.aes-sha2", "ciphertext": "opaque"}},
	}

	msg := messageFromEvent(testRoomID, evt)

	if msg.Sender != "@bob:example.com" {
		t.Errorf("Sender = %q, want it read from the envelope", msg.Sender)
	}
	if msg.MsgType != "" || msg.Body != "" || msg.FormattedBody != "" {
		t.Errorf("content = %q %q %q, want none", msg.MsgType, msg.Body, msg.FormattedBody)
	}
}

func TestMessageImageURLReadsTheFetchedContent(t *testing.T) {
	msg := messageFromEvent(testRoomID, messageEvent(map[string]any{
		"msgtype": "m.image",
		"body":    "cat.png",
		"url":     "mxc://example.com/cat",
	}))

	url, encryptionInfo, fullContent, err := MessageImageURL(context.Background(), msg)
	if err != nil {
		t.Fatalf("MessageImageURL() error = %v", err)
	}
	if url != "mxc://example.com/cat" || encryptionInfo != nil || fullContent["body"] != "cat.png" {
		t.Errorf("MessageImageURL() = %q, %v, %v", url, encryptionInfo, fullContent)
	}
}

func TestMessageImageURLRejectsText(t *testing.T) {
	msg := messageFromEvent(testRoomID, messageEvent(map[string]any{"msgtype": "m.text", "body": "hi"}))

	if _, _, _, err := MessageImageURL(context.Background(), msg); err == nil {
		t.Error("MessageImageURL() on a text message returned no error")
	}
}

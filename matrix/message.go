package matrix

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// Message is a room message fetched from the homeserver, decrypted if it was encrypted
type Message struct {
	RoomID    string
	EventID   string
	Sender    string
	Timestamp time.Time

	// MsgType, Body and FormattedBody are empty when the event isn't a message or its content could
	// not be decrypted. Body and FormattedBody hold only what the sender wrote: the quote of an
	// earlier message that a reply may open with is removed.
	MsgType       string
	Body          string
	FormattedBody string
	// RelatesTo is where the message sits: the message it replies to, and the thread it is in. Nil
	// for a message that relates to no other, or whose content couldn't be read.
	RelatesTo *event.RelatesTo

	// content is the event content as sent, for the media details of an image
	content map[string]any
}

// Caption returns the caption of an image: its body, when the message also names the file
// separately. Without a separate file name, the body is the file's name.
func (m *Message) Caption() string {
	fileName, _ := m.content["filename"].(string)
	if fileName == "" || m.Body == fileName {
		return ""
	}
	return m.Body
}

// FetchMessage fetches a single event, decrypting it if necessary. It is one round trip however
// the caller uses the message: who sent it, what it says, and where to download an image from.
//
// An event whose content can't be decrypted is still returned, just without the content. The
// sender is part of the unencrypted envelope, and telling who a reply went to needs nothing more.
func FetchMessage(ctx context.Context, roomID, eventID string) (*Message, error) {
	evt, err := client.GetEvent(ctx, id.RoomID(roomID), id.EventID(eventID))
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Str("event_id", eventID).Msg("Failed to fetch event")
		return nil, fmt.Errorf("failed to fetch event: %w", err)
	}

	if evt.Type == event.EventEncrypted {
		if err := evt.Content.ParseRaw(evt.Type); err != nil {
			log.Error().Ctx(ctx).Err(err).
				Str("room_id", roomID).
				Str("event_id", eventID).
				Msg("Failed to parse encrypted event content")
		}

		decryptedEvt, err := olmMachine.DecryptMegolmEvent(ctx, evt)
		if err != nil {
			log.Error().Ctx(ctx).Err(err).
				Str("room_id", roomID).
				Str("event_id", eventID).
				Msg("Failed to decrypt event")
			// Still encrypted, so only the envelope is read from it below
		} else {
			evt = decryptedEvt
		}
	}

	return messageFromEvent(roomID, evt), nil
}

// MessageFromEvent reads a message event as it arrived through sync, decrypted if it was encrypted,
// such as an image whose caption addressed the bot
func MessageFromEvent(evt *event.Event) *Message {
	return messageFromEvent(evt.RoomID.String(), evt)
}

// messageFromEvent reads a fetched event. Anything but a readable message, such as another event
// type or content that is still encrypted, yields only the envelope: who sent it, and when.
func messageFromEvent(roomID string, evt *event.Event) *Message {
	msg := &Message{
		RoomID:    roomID,
		EventID:   evt.ID.String(),
		Sender:    evt.Sender.String(),
		Timestamp: time.UnixMilli(evt.Timestamp),
	}
	if evt.Type != event.EventMessage {
		return msg
	}

	content := evt.Content.Raw
	msg.content = content
	msg.MsgType, _ = content["msgtype"].(string)
	msg.Body, _ = content["body"].(string)
	msg.FormattedBody, _ = content["formatted_body"].(string)
	msg.RelatesTo = relationOf(content)

	// A reply may open with a quote of the message it replies to. Whoever referred to this message
	// did not refer to that one, so only this message's own words are kept.
	if isReply(content) {
		msg.Body = StripReplyFallback(msg.Body)
		msg.FormattedBody = StripMxReply(msg.FormattedBody)
	}

	return msg
}

// relationOf reads the relation of event content, nil if it has none
func relationOf(content map[string]any) *event.RelatesTo {
	raw, ok := content["m.relates_to"]
	if !ok {
		return nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var rel event.RelatesTo
	if err := json.Unmarshal(encoded, &rel); err != nil {
		return nil
	}
	return &rel
}

// isReply reports whether event content carries a reply relation
func isReply(content map[string]any) bool {
	relatesTo, _ := content["m.relates_to"].(map[string]any)
	_, ok := relatesTo["m.in_reply_to"].(map[string]any)
	return ok
}

// IsFirstThreadReply reports whether an event is the earliest reply in the thread under root.
//
// A thread message's own relation can't answer that. It names the root either way, and its reply
// fallback points at whatever the sender's client took to be the latest message in the thread.
// The server knows the actual order, so it is asked for the thread's oldest reply.
func IsFirstThreadReply(ctx context.Context, roomID, rootID, eventID string) (bool, error) {
	resp, err := client.GetRelations(ctx, id.RoomID(roomID), id.EventID(rootID), &mautrix.ReqGetRelations{
		RelationType: event.RelThread,
		Dir:          mautrix.DirectionForward,
		Limit:        1,
	})
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", roomID).
			Str("root_event_id", rootID).
			Str("event_id", eventID).
			Msg("Failed to get the first reply of a thread")
		return false, fmt.Errorf("failed to get thread relations: %w", err)
	}

	return len(resp.Chunk) > 0 && resp.Chunk[0].ID.String() == eventID, nil
}

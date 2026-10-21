package chat

import (
	"context"
	"time"

	"github.com/Scrin/siikabot/db"
	"github.com/Scrin/siikabot/matrix"
	"github.com/Scrin/siikabot/metrics"
	"github.com/rs/zerolog/log"
)

// Notes for the model about a message the turn refers to but can't show it
const (
	unreadableReplyNote = "Note: This message is a reply to another message, but I couldn't retrieve the content of that message."
	unreadableLinkNote  = "Note: This message links to another message in the room, but I couldn't retrieve the content of that message."
)

// Notes for the model about an image that couldn't be attached
const (
	unusableOwnImageNote   = "Note: The user sent an image, but I couldn't process it. Please make sure the image is accessible and try again."
	unusableReplyImageNote = "Note: The user replied to an image, but I couldn't process it. Please make sure the image is accessible and try again."
	unusableLinkImageNote  = "Note: The user linked to an image, but I couldn't process it. Please make sure the image is accessible and try again."
)

// references is what a turn gets of the messages its trigger explicitly refers to, and of the image
// the trigger is if it is one: the quotes and the images kept with the turn, and notes for the model
// about what it can't be shown
type references struct {
	replyTo *db.QuotedMessage
	links   []db.QuotedMessage
	// images are the images the turn shows the model, prepared for it, at most one for each message
	images []db.ChatImage
	notes  []string
	// imageTime is how long getting the images ready took, downloading and preparing them
	imageTime time.Duration
}

// referencedMessages works out what a turn gets of the messages its trigger explicitly refers to,
// and of the image it is if it is one. withReply is false when the message it replies to isn't to
// be quoted: the first message of a thread whose opening turn is in the context already.
//
// Apart from messages addressed to the bot, these are the only room content that reaches the model.
// They were fetched when the message was routed, so nothing here goes back to the room for more.
func referencedMessages(ctx context.Context, trigger Trigger, room roomInfo, withReply bool) references {
	// Images are attached in the order the turn shows what they belong to: the replied-to message,
	// the linked ones, then the message's own image
	var refs references

	if withReply && trigger.ReplyToEventID != "" {
		log.Debug().Ctx(ctx).
			Str("room_id", trigger.RoomID).
			Str("reply_event_id", trigger.ReplyToEventID).
			Msg("Message refers to another message")

		if quote := quoteOf(ctx, trigger.RoomID, trigger.ReplyTo, room); quote == nil {
			refs.notes = append(refs.notes, unreadableReplyNote)
		} else {
			refs.replyTo = quote
			if quote.Kind == db.QuoteImage {
				refs.attach(ctx, trigger.RoomID, trigger.ReplyTo, unusableReplyImageNote)
			}
		}
	}

	unreadableLink := false
	for _, link := range trigger.Links {
		quote := quoteOf(ctx, trigger.RoomID, link.Message, room)
		if quote == nil {
			unreadableLink = true
			continue
		}
		refs.links = append(refs.links, *quote)
		if quote.Kind == db.QuoteImage {
			refs.attach(ctx, trigger.RoomID, link.Message, unusableLinkImageNote)
		}
	}
	if unreadableLink {
		refs.notes = append(refs.notes, unreadableLinkNote)
	}

	if trigger.Image != nil {
		refs.attach(ctx, trigger.RoomID, trigger.Image, unusableOwnImageNote)
	}

	return refs
}

// quoteOf is a message as a turn quotes it, or nil if nothing of it is readable: it couldn't be
// fetched, its sender couldn't see it, its content couldn't be decrypted, it was deleted, or the
// event isn't a message at all
func quoteOf(ctx context.Context, roomID string, msg *matrix.Message, room roomInfo) *db.QuotedMessage {
	if msg == nil {
		// Already logged where it was fetched
		return nil
	}

	isImage := msg.MsgType == "m.image"
	if !isImage && msg.Body == "" {
		log.Debug().Ctx(ctx).
			Str("room_id", roomID).
			Str("event_id", msg.EventID).
			Msg("Referenced message has no readable content")
		return nil
	}

	quote := &db.QuotedMessage{
		EventID:    msg.EventID,
		Sender:     msg.Sender,
		SenderName: room.nameOf(ctx, msg.Sender),
		SentAt:     msg.Timestamp,
	}
	if isImage {
		quote.Kind = db.QuoteImage
		quote.Body = capQuote(msg.Caption())
		return quote
	}

	log.Debug().Ctx(ctx).
		Str("room_id", roomID).
		Str("event_id", msg.EventID).
		Str("content", msg.Body).
		Msg("Including referenced message in conversation")

	quote.Kind = db.QuoteText
	quote.Body = capQuote(msg.Body)
	return quote
}

// attach downloads the image of a message and prepares it for the model, which the turn then shows
// it to. An image that can't be shown leaves a note for the model instead, failureNote if it couldn't
// be downloaded or prepared.
func (r *references) attach(ctx context.Context, roomID string, msg *matrix.Message, failureNote string) {
	start := time.Now()
	image, ok, note := imageOf(ctx, roomID, msg, failureNote)
	r.imageTime += time.Since(start)
	if note != "" {
		r.notes = append(r.notes, note)
	}
	if !ok {
		return
	}
	r.images = append(r.images, image)
	metrics.RecordChatImageProcessed()
}

// imageOf downloads the image of a message and prepares it for the model (see prepareImage). When
// it can't be shown, ok is false and there may be a note for the model: failureNote if the image
// couldn't be downloaded or prepared.
func imageOf(ctx context.Context, roomID string, msg *matrix.Message, failureNote string) (image db.ChatImage, ok bool, note string) {
	// Get the image URL, encryption info, and full content
	imageURL, encryptionInfo, fullContent, err := matrix.MessageImageURL(ctx, msg)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", roomID).
			Str("event_id", msg.EventID).
			Msg("Failed to get image URL from a message")
		return db.ChatImage{}, false, ""
	}

	data, err := matrix.DownloadImage(ctx, imageURL, encryptionInfo, fullContent)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", roomID).
			Str("image_url", imageURL).
			Bool("is_encrypted", encryptionInfo != nil).
			Msg("Failed to download an image")
		return db.ChatImage{}, false, failureNote
	}

	image, err = prepareImage(data)
	if err != nil {
		log.Warn().Ctx(ctx).Err(err).
			Str("room_id", roomID).
			Str("event_id", msg.EventID).
			Int("size_bytes", len(data)).
			Msg("Failed to prepare an image for the model")
		return db.ChatImage{}, false, failureNote
	}
	image.EventID = msg.EventID

	log.Debug().Ctx(ctx).
		Str("room_id", roomID).
		Str("event_id", msg.EventID).
		Str("image_url", imageURL).
		Bool("is_encrypted", encryptionInfo != nil).
		Int("downloaded_bytes", len(data)).
		Int("prepared_bytes", len(image.Data)).
		Str("mime_type", image.MIMEType).
		Int("width", image.Width).
		Int("height", image.Height).
		Msg("Attaching an image to the chat turn")

	return image, true, ""
}

package chat

import (
	"context"
	"strings"

	"github.com/Scrin/siikabot/db"
	"github.com/Scrin/siikabot/matrix"
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
// the trigger is if it is one: the quotes kept with the turn, the images attached to this turn only,
// and notes for the model about what it can't be shown
type references struct {
	replyTo *db.QuotedMessage
	links   []db.QuotedMessage
	// images are the images attached to the turn, as data URLs, and attached says whose they are,
	// by event ID
	images   []string
	attached map[string]bool
	notes    []string
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
	refs := references{attached: make(map[string]bool)}

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

// attach downloads the image of a message and attaches it to the turn. An image that can't be
// attached leaves a note for the model instead, failureNote if it couldn't be downloaded.
func (r *references) attach(ctx context.Context, roomID string, msg *matrix.Message, failureNote string) {
	imageDataURL, note := imageOf(ctx, roomID, msg, failureNote)
	if note != "" {
		r.notes = append(r.notes, note)
	}
	if imageDataURL == "" {
		return
	}
	r.images = append(r.images, imageDataURL)
	r.attached[msg.EventID] = true
}

// imageOf downloads the image of a message, returning it as a data URL ready to attach. When it
// can't be attached, the data URL is empty and there may be a note for the model: failureNote if
// the download failed.
func imageOf(ctx context.Context, roomID string, msg *matrix.Message, failureNote string) (string, string) {
	// Get the image URL, encryption info, and full content
	imageURL, encryptionInfo, fullContent, err := matrix.MessageImageURL(ctx, msg)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", roomID).
			Str("event_id", msg.EventID).
			Msg("Failed to get image URL from a message")
		return "", ""
	}

	// Download the image and convert to base64
	base64ImageURL, err := matrix.DownloadImageAsBase64(ctx, imageURL, encryptionInfo, fullContent)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", roomID).
			Str("image_url", imageURL).
			Bool("is_encrypted", encryptionInfo != nil).
			Msg("Failed to download and convert image to base64")
		return "", failureNote
	}

	log.Debug().Ctx(ctx).
		Str("room_id", roomID).
		Str("event_id", msg.EventID).
		Str("image_url", imageURL).
		Bool("is_encrypted", encryptionInfo != nil).
		Msg("Attaching an image to the chat turn")

	return usableImage(ctx, roomID, base64ImageURL)
}

// maxImageBytes is the largest image that is attached to a request
const maxImageBytes = 5 * 1024 * 1024

// usableImage checks that a downloaded image can be attached: a well-formed data URL of at most
// maxImageBytes. Returns the data URL to attach, or an empty one with a note for the model.
func usableImage(ctx context.Context, roomID, base64ImageURL string) (string, string) {
	// Ensure the base64ImageURL is properly formatted
	if !strings.HasPrefix(base64ImageURL, "data:image/") {
		// Log a prefix of the URL for debugging, but be careful of index out of range
		urlPrefix := base64ImageURL
		if len(base64ImageURL) > 30 {
			urlPrefix = base64ImageURL[:30] + "..."
		}

		log.Warn().Ctx(ctx).
			Str("room_id", roomID).
			Str("base64_url_prefix", urlPrefix).
			Msg("Image URL is not properly formatted, attempting to fix")

		// Try to extract the content type and base64 data
		if strings.Contains(base64ImageURL, ";base64,") {
			parts := strings.SplitN(base64ImageURL, ";base64,", 2)
			if len(parts) == 2 {
				contentType := parts[0]
				if !strings.HasPrefix(contentType, "data:") {
					contentType = "data:" + contentType
				}
				if !strings.HasPrefix(contentType, "data:image/") {
					contentType = "data:image/png"
				}
				base64Data := parts[1]
				base64ImageURL = contentType + ";base64," + base64Data

				// Log a prefix of the fixed URL for debugging, but be careful of index out of range
				fixedUrlPrefix := base64ImageURL
				if len(base64ImageURL) > 30 {
					fixedUrlPrefix = base64ImageURL[:30] + "..."
				}

				log.Debug().Ctx(ctx).
					Str("room_id", roomID).
					Str("fixed_url_prefix", fixedUrlPrefix).
					Msg("Fixed image URL format")
			}
		}
	}

	parts := strings.SplitN(base64ImageURL, ";base64,", 2)
	if len(parts) != 2 {
		log.Error().Ctx(ctx).
			Str("room_id", roomID).
			Msg("Image URL does not contain valid base64 data, skipping image")
		return "", ""
	}

	// Base64 encoding increases size by ~33%, so this approximates the decoded size
	estimatedSize := len(parts[1]) * 3 / 4
	if estimatedSize > maxImageBytes {
		log.Warn().Ctx(ctx).
			Str("room_id", roomID).
			Int("estimated_size_bytes", estimatedSize).
			Int("max_size_bytes", maxImageBytes).
			Msg("Image is too large, skipping image attachment")
		return "", "Note: An image was attached to this message, but it was too large to process (>5MB)."
	}

	return base64ImageURL, ""
}

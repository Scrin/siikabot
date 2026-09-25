package bot

import (
	"context"
	"time"

	"github.com/Scrin/siikabot/commands/chat"
	"github.com/Scrin/siikabot/config"
	"github.com/Scrin/siikabot/constants"
	"github.com/Scrin/siikabot/db"
	"github.com/Scrin/siikabot/matrix"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel/attribute"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// routeToChat starts a chat turn for a message if it addresses the bot, returning what it was
// handled as, or an empty command if it doesn't address the bot. body and formattedBody are what the
// sender wrote: a text message's text, or an image's caption, reply quote included. image is the
// message itself when it is an image, nil for text.
func routeToChat(ctx context.Context, evt *event.Event, attrs []attribute.KeyValue, body, formattedBody string, image *matrix.Message) constants.Command {
	roomID, sender, eventID := evt.RoomID.String(), evt.Sender.String(), evt.ID.String()

	// The event the message explicitly refers to decides both whether it counts as a reply to the
	// bot and what reply context the chat turn gets, so it is worked out once, here
	rel := evt.Content.AsMessage().RelatesTo
	replyToID, replyTo := repliedTo(ctx, roomID, sender, eventID, rel)
	isReplyToBot := replyTo != nil && replyTo.Sender == config.UserID

	// Check if the message addresses the bot: an explicit mention of the bot in m.mentions, a
	// message that opens by naming the bot, or a reply to the bot
	isMentioned := mentionsBotExplicitly(evt.Content.Raw, config.UserID)
	prefixedMsg, isPrefixed := stripBotNamePrefix(body, formattedBody, config.UserID, botNames(ctx, roomID)...)
	addressed := isMentioned || isPrefixed || isReplyToBot

	// A reply to the bot that neither pills it nor opens with its name addresses it only by being a
	// reply. The bot being in its m.mentions doesn't say otherwise (see mention.go).
	byReplyOnly := isReplyToBot && !isPrefixed && !pillsBot(formattedBody, config.UserID)

	// Such a reply that opens by addressing someone else is for them, and like any other message
	// that isn't for the bot, it only counts as unseen
	if byReplyOnly && addressesSomeoneElse(body, formattedBody, config.UserID, sender, roomMembers(ctx, roomID)) {
		log.Debug().Ctx(ctx).
			Str("room_id", roomID).
			Str("event_id", eventID).
			Msg("Skipping a reply to the bot that addresses someone else")
		addressed = false
	}
	if !addressed {
		return ""
	}

	ownBody, ownFormatted := ownText(body, formattedBody, rel)

	// Only the leading address is dropped from the message. A mention anywhere else is part of what
	// the sender wrote and reads better left alone.
	text := ownBody
	if isPrefixed {
		text = prefixedMsg
		// Nothing but the bot's name: the sender is getting our attention rather than asking
		// anything, so let the model see the name it was called by
		if text == "" {
			text = ownBody
		}
	}

	startChatTurn(ctx, attrs, chatStart{
		roomID:       roomID,
		sender:       sender,
		eventID:      eventID,
		sentAt:       time.UnixMilli(evt.Timestamp),
		threadRootID: rel.GetThreadParent().String(),
		text:         text,
		ownBody:      ownBody,
		ownFormatted: ownFormatted,
		mentions:     mentionedUserIDs(evt.Content.Raw),
		replyToID:    replyToID,
		replyTo:      replyTo,
		image:        image,
		byReplyOnly:  byReplyOnly,
	})
	if isReplyToBot {
		return constants.CommandReply
	}
	return constants.CommandMention
}

// routeEdit handles an edit of a message, returning what it was handled as.
//
// The chat history follows the edit, so that a turn the message started, or a quote of it, reads as
// edited. The edit itself is answered only if it newly mentions the bot, and then as the message now
// reads: clients list in an edit's own m.mentions only the users the edit adds. Any other edit
// changes a message that was already handled when it was sent.
func routeEdit(ctx context.Context, evt *event.Event, attrs []attribute.KeyValue) constants.Command {
	roomID, sender := evt.RoomID.String(), evt.Sender.String()
	content := evt.Content.AsMessage()
	originalID := content.RelatesTo.GetReplaceID().String()
	edited := content.NewContent
	if edited == nil {
		// An edit without its new content changes nothing
		return ""
	}

	// The new text, or the new caption of an image. It carries no reply quote, since an edit's new
	// content never includes one.
	body, formattedBody := edited.Body, edited.FormattedBody
	if edited.MsgType == event.MsgImage {
		body, formattedBody = edited.GetCaption(), edited.GetFormattedCaption()
	}
	text := body
	if rest, ok := stripBotNamePrefix(body, formattedBody, config.UserID, botNames(ctx, roomID)...); ok && rest != "" {
		text = rest
	}
	chat.FollowEdit(ctx, roomID, originalID, sender, text, body)

	if !mentionsBotExplicitly(evt.Content.Raw, config.UserID) || body == "" {
		return ""
	}

	// The message as it was sent says where it is: the message it replies to, and its thread
	original, err := matrix.FetchMessage(ctx, roomID, originalID)
	if err != nil {
		// Already logged
		return ""
	}
	if original.Sender != sender {
		// Clients ignore an edit of someone else's message, and so does the bot
		log.Warn().Ctx(ctx).
			Str("room_id", roomID).
			Str("event_id", originalID).
			Str("original_sender", original.Sender).
			Msg("Ignoring an edit of someone else's message")
		return ""
	}
	if answered, err := db.HasChatTurn(ctx, roomID, originalID); err != nil || answered {
		// A message the bot has answered already is only brought up to date, above
		return ""
	}

	var mentions []string
	if edited.Mentions != nil {
		mentions = make([]string, 0, len(edited.Mentions.UserIDs))
		for _, userID := range edited.Mentions.UserIDs {
			mentions = append(mentions, userID.String())
		}
	}
	var image *matrix.Message
	if edited.MsgType == event.MsgImage && original.MsgType == "m.image" {
		image = original
	}

	replyToID, replyTo := repliedTo(ctx, roomID, sender, originalID, original.RelatesTo)
	startChatTurn(ctx, attrs, chatStart{
		roomID:       roomID,
		sender:       sender,
		eventID:      originalID,
		sentAt:       time.UnixMilli(evt.Timestamp),
		threadRootID: original.RelatesTo.GetThreadParent().String(),
		text:         text,
		ownBody:      body,
		ownFormatted: formattedBody,
		mentions:     mentions,
		replyToID:    replyToID,
		replyTo:      replyTo,
		image:        image,
		wasUnseen:    true,
	})
	return constants.CommandMention
}

// repliedTo resolves the message a message explicitly replies to: its event, and the message as
// the sender may see it (see replyTarget and fetchReferenced)
func repliedTo(ctx context.Context, roomID, sender, eventID string, rel *event.RelatesTo) (id.EventID, *matrix.Message) {
	replyToID := replyTarget(rel, func(root id.EventID) bool {
		first, err := matrix.IsFirstThreadReply(ctx, roomID, root.String(), eventID)
		// Already logged. Without an answer the root stays out of the context.
		return err == nil && first
	})
	if replyToID == "" {
		return "", nil
	}
	return replyToID, fetchReferenced(ctx, roomID, sender, replyToID.String())
}

// chatStart is a message that addressed the bot, as routing worked it out
type chatStart struct {
	roomID, sender string
	// eventID is the message the turn is about, and sentAt when it was sent or last edited
	eventID      string
	sentAt       time.Time
	threadRootID string
	// text is what the sender asked, and ownBody and ownFormatted what they wrote, all without a
	// reply's quote. Unlike the other two, text also leaves out a leading address to the bot.
	text, ownBody, ownFormatted string
	// mentions are the users in the message's m.mentions, nil if it has none
	mentions  []string
	replyToID id.EventID
	replyTo   *matrix.Message
	// image is the message itself when it is an image, nil for text
	image *matrix.Message
	// wasUnseen says the message was counted as unseen when it was sent, as a message that only
	// addressed the bot once it was edited was
	wasUnseen bool
	// byReplyOnly says the message addressed the bot only by replying to it
	byReplyOnly bool
}

// startChatTurn hands a message that addressed the bot to the chat, which answers it in a turn of
// its own
func startChatTurn(ctx context.Context, attrs []attribute.KeyValue, msg chatStart) {
	trigger := chat.Trigger{
		RoomID:         msg.roomID,
		Sender:         msg.sender,
		EventID:        msg.eventID,
		Timestamp:      msg.sentAt,
		Body:           msg.text,
		FormattedBody:  msg.ownFormatted,
		Mentions:       msg.mentions,
		ThreadRootID:   msg.threadRootID,
		ReplyToEventID: msg.replyToID.String(),
		ReplyTo:        msg.replyTo,
		Links:          linkedMessages(ctx, msg.roomID, msg.sender, msg.ownBody, msg.ownFormatted, msg.eventID, msg.replyToID.String()),
		Image:          msg.image,
		ByReplyOnly:    msg.byReplyOnly,
	}

	// How many messages the bot didn't see since the previous one addressed to it. Taken here, as
	// messages arrive and in their order, so the count covers exactly the gap before this one; a
	// turn waiting for the room would otherwise count what came after. A failure is already logged,
	// and the count is only ever a hint, so it is left at 0.
	trigger.UnseenBefore, _ = db.TakeUnseenMessages(ctx, msg.roomID)
	if msg.wasUnseen && trigger.UnseenBefore > 0 {
		trigger.UnseenBefore--
	}

	traced(ctx, "chat.mention", attrs, func(ctx context.Context) {
		chat.HandleMention(ctx, trigger)
	})
}

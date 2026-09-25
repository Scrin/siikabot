package bot

import (
	"context"
	"strings"
	"time"

	"github.com/Scrin/siikabot/aigateway"
	"github.com/Scrin/siikabot/api"
	"github.com/Scrin/siikabot/auth"
	authcmd "github.com/Scrin/siikabot/commands/auth"
	"github.com/Scrin/siikabot/commands/chat"
	"github.com/Scrin/siikabot/commands/federation"
	"github.com/Scrin/siikabot/commands/ping"
	"github.com/Scrin/siikabot/commands/remind"
	"github.com/Scrin/siikabot/commands/stats"
	"github.com/Scrin/siikabot/commands/traceroute"
	"github.com/Scrin/siikabot/config"
	"github.com/Scrin/siikabot/constants"
	"github.com/Scrin/siikabot/db"
	"github.com/Scrin/siikabot/logging"
	"github.com/Scrin/siikabot/matrix"
	"github.com/Scrin/siikabot/metrics"
	"github.com/Scrin/siikabot/tracing"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel/trace"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

func handleTextEvent(ctx context.Context, evt *event.Event) {
	if evt.Sender.String() == config.UserID {
		return
	}

	ctx = logging.ContextWithStr(ctx, "msg_room_id", evt.RoomID.String())
	ctx = logging.ContextWithStr(ctx, "msg_sender", evt.Sender.String())
	ctx = logging.ContextWithStr(ctx, "msg_event_id", evt.ID.String())

	msgtype := ""
	if m, ok := evt.Content.Raw["msgtype"].(string); ok {
		msgtype = m
	}

	attrs := messageAttrs(evt.RoomID.String(), evt.Sender.String(), evt.ID.String())

	// Everything from here to the dispatch below is real work — a reply lookup against the
	// homeserver, mention detection, counting the message — and until this span existed it all
	// happened before any span was open. That had two costs: its latency was invisible, because
	// the handler span only starts once the routing decision is already made, and every call it
	// made produced a parentless span of its own. Those orphans were the bulk of the junk traces
	// in Tempo.
	//
	// The handler spans started below are children of this one and outlive it, since they run
	// asynchronously. That is legal and reads correctly: this span is the routing, not the work.
	ctx, routeSpan := tracer.Start(ctx, "message.route",
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(attrs...))
	defer routeSpan.End()

	routedToChat := false
	if msgtype == "m.text" {
		msg := evt.Content.Raw["body"].(string)

		// Track message stats asynchronously
		traced(ctx, "stats.message", attrs, func(ctx context.Context) {
			db.UpdateMessageStats(ctx, evt.RoomID.String(), evt.Sender.String(), msg)
		})
		traced(ctx, "stats.room_daily", attrs, func(ctx context.Context) {
			db.UpdateRoomDailyStats(ctx, evt.RoomID.String(), msg)
		})

		format, _ := evt.Content.Raw["format"].(string)
		formattedBody, _ := evt.Content.Raw["formatted_body"].(string)
		cmd := constants.Command(strings.Split(msg, " ")[0])
		isCommand := true

		switch cmd {
		case constants.CommandPing:
			traced(ctx, "command.ping", attrs, func(ctx context.Context) {
				ping.Handle(ctx, evt.RoomID.String(), msg)
			})
		case constants.CommandTraceroute:
			traced(ctx, "command.traceroute", attrs, func(ctx context.Context) {
				traceroute.Handle(ctx, evt.RoomID.String(), msg)
			})
		case constants.CommandRemind:
			traced(ctx, "command.remind", attrs, func(ctx context.Context) {
				remind.Handle(ctx, evt.RoomID.String(), evt.Sender.String(), msg, format, formattedBody)
			})
		case constants.CommandChat:
			traced(ctx, "command.chat", attrs, func(ctx context.Context) {
				chat.Handle(ctx, evt.RoomID.String(), evt.Sender.String(), msg)
			})
		case constants.CommandServers:
			traced(ctx, "command.servers", attrs, func(ctx context.Context) {
				federation.Handle(ctx, evt.RoomID.String(), msg)
			})
		case constants.CommandAuth:
			traced(ctx, "command.auth", attrs, func(ctx context.Context) {
				authcmd.Handle(ctx, evt.RoomID.String(), evt.Sender.String(), msg)
			})
		case constants.CommandStats:
			traced(ctx, "command.stats", attrs, func(ctx context.Context) {
				stats.Handle(ctx, evt.RoomID.String(), evt.Sender.String(), msg)
			})
		default:
			isCommand = false

			// The event the message explicitly refers to decides both whether it counts as a reply
			// to the bot and what reply context the chat turn gets, so it is worked out once, here
			rel := evt.Content.AsMessage().RelatesTo
			replyToID := replyTarget(rel, func(root id.EventID) bool {
				first, err := matrix.IsFirstThreadReply(ctx, evt.RoomID.String(), root.String(), evt.ID.String())
				// Already logged. Without an answer the root stays out of the context.
				return err == nil && first
			})

			// Fetched once for everything that needs it: the sender for the check below, and the
			// content for the chat turn
			var replyTo *matrix.Message
			if replyToID != "" {
				if fetched, err := matrix.FetchMessage(ctx, evt.RoomID.String(), replyToID.String()); err == nil {
					replyTo = fetched
				}
			}
			isReplyToBot := replyTo != nil && replyTo.Sender == config.UserID

			// Check if the message addresses the bot: an explicit mention of the bot in
			// m.mentions, or a message that opens by naming the bot
			isMentioned := mentionsBotExplicitly(evt.Content.Raw, config.UserID)
			prefixedMsg, isPrefixed := stripBotNamePrefix(msg, formattedBody, config.UserID, botNames(ctx, evt.RoomID.String())...)

			if isMentioned || isPrefixed || isReplyToBot {
				body, formatted := ownText(msg, formattedBody, rel)

				// Only the leading address is dropped from the message. A mention anywhere else is
				// part of what the sender wrote and reads better left alone.
				chatMsg := body
				if isPrefixed {
					chatMsg = prefixedMsg
					// Nothing but the bot's name: the sender is getting our attention rather than
					// asking anything, so let the model see the name it was called by
					if chatMsg == "" {
						chatMsg = body
					}
				}

				trigger := chat.Trigger{
					RoomID:         evt.RoomID.String(),
					Sender:         evt.Sender.String(),
					EventID:        evt.ID.String(),
					Timestamp:      time.UnixMilli(evt.Timestamp),
					Body:           chatMsg,
					FormattedBody:  formatted,
					Mentions:       mentionedUserIDs(evt.Content.Raw),
					ThreadRootID:   rel.GetThreadParent().String(),
					ReplyToEventID: replyToID.String(),
					ReplyTo:        replyTo,
				}

				// How many messages the bot didn't see since the previous one addressed to it. Taken
				// here, as messages arrive and in their order, so the count covers exactly the gap
				// before this one; a turn waiting for the room would otherwise count what came after.
				// A failure is already logged, and the count is only ever a hint, so it is left at 0.
				trigger.UnseenBefore, _ = db.TakeUnseenMessages(ctx, evt.RoomID.String())

				traced(ctx, "chat.mention", attrs, func(ctx context.Context) {
					chat.HandleMention(ctx, trigger)
				})
				routedToChat = true
				isCommand = true
				cmd = constants.CommandMention
				if isReplyToBot {
					cmd = constants.CommandReply
				}
			}
		}
		if isCommand {
			matrix.MarkRead(ctx, evt.RoomID.String(), evt.ID.String())
			log.Debug().
				Str("command", string(cmd)).
				Str("room_id", evt.RoomID.String()).
				Str("sender", evt.Sender.String()).
				Msg("Handled command")
			metrics.RecordCommandHandled(cmd)
		}
	}

	// Every other message from someone else is one the chat model never sees, and only how many
	// there were is kept. An edit changes a message that was already counted.
	if !routedToChat && evt.Content.AsMessage().RelatesTo.GetReplaceID() == "" {
		if err := db.CountUnseenMessage(ctx, evt.RoomID.String()); err != nil {
			// Already logged. The count is only ever a hint.
			_ = err
		}
	}
}

// botNames returns the names a message can address the bot by: its global display name, and its
// display name in the room when it has a different one there
func botNames(ctx context.Context, roomID string) []string {
	names := []string{matrix.GetDisplayName(ctx, config.UserID)}
	if room, err := matrix.GetRoom(ctx, roomID); err == nil {
		if member, ok := room.Member(config.UserID); ok && member.DisplayName != "" {
			names = append(names, member.DisplayName)
		}
	}
	return names
}

// handleRedactionEvent removes a redacted message from the chat history. The redacted event is in
// the content from room version 11 on, and at the top level of the event before that.
//
// Handled right here in the sync loop rather than in a goroutine: a chat turn for the redacted
// message may already be running, and it stops the moment the redaction is recorded. Redactions are
// rare enough that the wait costs nothing.
func handleRedactionEvent(ctx context.Context, evt *event.Event) {
	redacts := evt.Redacts
	if redacts == "" {
		redacts = evt.Content.AsRedaction().Redacts
	}
	if redacts == "" {
		return
	}

	attrs := messageAttrs(evt.RoomID.String(), evt.Sender.String(), evt.ID.String())
	tracing.Run(ctx, tracer, "chat.forget", func(ctx context.Context) {
		chat.ForgetEvent(ctx, evt.RoomID.String(), redacts.String())
	}, trace.WithSpanKind(trace.SpanKindConsumer), trace.WithAttributes(attrs...))
}

func handleMemberEvent(ctx context.Context, evt *event.Event) {
	if evt.Content.Raw["membership"] == "invite" && evt.GetStateKey() == config.UserID {
		matrix.JoinRoom(ctx, evt.RoomID.String())
		log.Info().
			Str("room_id", evt.RoomID.String()).
			Str("inviter", evt.Sender.String()).
			Msg("Joined room from invite")
	}
}

func handleEvent(ctx context.Context, evt *event.Event, wasEncrypted bool) {
	switch evt.Type {
	case event.EventMessage:
		handleTextEvent(ctx, evt)
	case event.StateMember:
		handleMemberEvent(ctx, evt)
	case event.EventRedaction:
		handleRedactionEvent(ctx, evt)
	}
	subtype := ""
	if m, ok := evt.Content.Raw["msgtype"].(string); ok {
		subtype = m
	}
	metrics.RecordEventHandled(evt.Type.String(), subtype, wasEncrypted)
}

func Init(ctx context.Context) error {
	if err := db.Init(); err != nil {
		return err
	}
	if err := matrix.Init(ctx, handleEvent); err != nil {
		return err
	}

	resp, err := matrix.InitialSync(ctx)
	if err != nil {
		return err
	}
	for roomID := range resp.Rooms.Invite {
		matrix.JoinRoom(ctx, roomID.String())
		log.Info().
			Str("room_id", roomID.String()).
			Msg("Joined room during initial sync")
	}

	remind.Init(ctx)
	chat.Init(ctx)
	aigateway.StartLogPoller(ctx)
	auth.Init()
	api.Init()
	initHTTP()

	return nil
}

func Run(ctx context.Context) error {
	return matrix.Sync(ctx)
}

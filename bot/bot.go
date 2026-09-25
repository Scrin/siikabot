package bot

import (
	"context"
	"strings"

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
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel/trace"
	"maunium.net/go/mautrix/event"
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

	if msgtype == "m.text" && evt.Sender.String() != config.UserID {
		msg := evt.Content.Raw["body"].(string)

		attrs := messageAttrs(evt.RoomID.String(), evt.Sender.String(), evt.ID.String())

		// Everything from here to the dispatch below is real work — a reply lookup against the
		// homeserver, mention detection — and until this span existed it all happened before any
		// span was open. That had two costs: its latency was invisible, because
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

			// Extract the m.relates_to field if it exists
			var relatesTo map[string]any
			if relates, ok := evt.Content.Raw["m.relates_to"].(map[string]any); ok {
				relatesTo = relates
			}

			// Check if the message is a reply to a message sent by the bot
			isReplyToBot := false
			if relatesTo != nil {
				if inReplyTo, ok := relatesTo["m.in_reply_to"].(map[string]any); ok {
					if replyEventID, ok := inReplyTo["event_id"].(string); ok {
						// Get the sender of the replied-to message
						repliedToSender, err := matrix.GetEventSender(ctx, evt.RoomID.String(), replyEventID)
						if err == nil && repliedToSender == config.UserID {
							isReplyToBot = true
						}
					}
				}
			}

			// Check if the message addresses the bot: an explicit mention of the bot in
			// m.mentions, or a message that opens by naming the bot
			isMentioned := mentionsBotExplicitly(evt.Content.Raw, config.UserID)
			botDisplayName := matrix.GetDisplayName(ctx, config.UserID)
			prefixedMsg, isPrefixed := stripBotNamePrefix(msg, formattedBody, config.UserID, botDisplayName)

			if isMentioned || isPrefixed || isReplyToBot {
				// Only the leading address is dropped from the message. A mention anywhere else is
				// part of what the sender wrote and reads better left alone.
				chatMsg := msg
				if isPrefixed {
					chatMsg = prefixedMsg
					// Nothing but the bot's name: the sender is getting our attention rather than
					// asking anything, so let the model see the name it was called by
					if chatMsg == "" {
						chatMsg = msg
					}
				}

				traced(ctx, "chat.mention", attrs, func(ctx context.Context) {
					chat.HandleMention(ctx, evt.RoomID.String(), evt.Sender.String(), chatMsg, evt.ID.String(), relatesTo)
				})
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

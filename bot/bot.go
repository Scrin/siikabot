package bot

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/Scrin/siikabot/commands/chat"
	"github.com/Scrin/siikabot/commands/grafana"
	"github.com/Scrin/siikabot/commands/ping"
	"github.com/Scrin/siikabot/commands/remind"
	"github.com/Scrin/siikabot/commands/ruuvi"
	"github.com/Scrin/siikabot/commands/traceroute"
	"github.com/Scrin/siikabot/db"
	"github.com/Scrin/siikabot/matrix"
	"github.com/Scrin/siikabot/metrics"
	"github.com/rs/zerolog/log"
	"maunium.net/go/mautrix/event"
)

var (
	adminUser string
)

func handleTextEvent(ctx context.Context, evt *event.Event) {
	content, _ := json.Marshal(evt.Content.Raw)
	log.Debug().RawJSON("content", content).Msg("Received text event")

	if evt.Sender.String() == matrix.GetUserID() {
		log.Debug().Str("sender", evt.Sender.String()).Msg("Skipping event from self")
		return
	}

	msgtype := ""
	if m, ok := evt.Content.Raw["msgtype"].(string); ok {
		msgtype = m
	}
	metrics.RecordEventHandled("m.room.message", msgtype)

	if msgtype == "m.text" && evt.Sender.String() != matrix.GetUserID() {
		msg := evt.Content.Raw["body"].(string)
		format, _ := evt.Content.Raw["format"].(string)
		formattedBody, _ := evt.Content.Raw["formatted_body"].(string)
		msgCommand := strings.Split(msg, " ")[0]
		isCommand := true

		switch msgCommand {
		case "!ping":
			ping.Handle(evt.RoomID.String(), msg)
		case "!traceroute":
			traceroute.Handle(evt.RoomID.String(), msg)
		case "!ruuvi":
			ruuvi.Handle(evt.RoomID.String(), evt.Sender.String(), msg)
		case "!grafana":
			grafana.Handle(evt.RoomID.String(), evt.Sender.String(), msg)
		case "!remind":
			remind.Handle(evt.RoomID.String(), evt.Sender.String(), msg, format, formattedBody)
		case "!chat":
			chat.Handle(evt.RoomID.String(), evt.Sender.String(), msg)
		default:
			isCommand = false
		}
		if isCommand {
			log.Debug().
				Str("command", msgCommand).
				Str("room_id", evt.RoomID.String()).
				Str("sender", evt.Sender.String()).
				Msg("Handled command")
			metrics.RecordCommandHandled(msgCommand)
		}
	}
}

func handleMemberEvent(ctx context.Context, evt *event.Event) {
	metrics.RecordEventHandled("m.room.member", "")

	if evt.Content.Raw["membership"] == "invite" && evt.GetStateKey() == matrix.GetUserID() {
		matrix.JoinRoom(evt.RoomID.String())
		log.Info().
			Str("room_id", evt.RoomID.String()).
			Str("inviter", evt.Sender.String()).
			Msg("Joined room from invite")
	}
}

func Run(homeserverURL, userID, accessToken, hookSecret, dataPath, admin, openrouterApiKey string) error {
	if err := db.Init(dataPath + "/siikabot.db"); err != nil {
		return err
	}
	if err := matrix.Init(homeserverURL, userID, accessToken); err != nil {
		return err
	}
	adminUser = admin

	resp := matrix.InitialSync()
	for roomID := range resp.Rooms.Invite {
		matrix.JoinRoom(roomID.String())
		log.Info().
			Str("room_id", roomID.String()).
			Msg("Joined room during initial sync")
	}

	remind.Init()
	chat.Init(openrouterApiKey)
	ruuvi.Init(admin)
	grafana.Init(admin)
	initHTTP(hookSecret)

	matrix.OnEvent("m.room.member", handleMemberEvent)
	matrix.OnEvent("m.room.message", handleTextEvent)

	return matrix.Sync()
}

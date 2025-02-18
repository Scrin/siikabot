package bot

import (
	"context"
	"log"
	"strings"

	"github.com/Scrin/siikabot/commands/chat"
	"github.com/Scrin/siikabot/commands/grafana"
	"github.com/Scrin/siikabot/commands/ping"
	"github.com/Scrin/siikabot/commands/remind"
	"github.com/Scrin/siikabot/commands/ruuvi"
	"github.com/Scrin/siikabot/commands/traceroute"
	"github.com/Scrin/siikabot/db"
	"github.com/Scrin/siikabot/matrix"
	"maunium.net/go/mautrix/event"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	adminUser string
)

func handleTextEvent(ctx context.Context, evt *event.Event) {
	log.Print(evt.Content.Raw)
	if evt.Sender.String() == matrix.GetUserID() {
		log.Print("Skipping event from self")
		return
	}
	msgtype := ""
	if m, ok := evt.Content.Raw["msgtype"].(string); ok {
		msgtype = m
	}
	metrics.eventsHandled.With(prometheus.Labels{"event_type": "m.room.message", "msg_type": msgtype}).Inc()
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
			metrics.commandsHandled.With(prometheus.Labels{"command": msgCommand}).Inc()
		}
	}
}

func handleMemberEvent(ctx context.Context, evt *event.Event) {
	metrics.eventsHandled.With(prometheus.Labels{"event_type": "m.room.member", "msg_type": ""}).Inc()
	if evt.Content.Raw["membership"] == "invite" && evt.GetStateKey() == matrix.GetUserID() {
		matrix.JoinRoom(evt.RoomID.String())
		log.Print("Joined room " + evt.RoomID.String())
	}
}

func Run(homeserverURL, userID, accessToken, hookSecret, dataPath, admin, openrouterApiKey string) error {
	initMetrics()
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
		log.Print("Joined room " + roomID.String())
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

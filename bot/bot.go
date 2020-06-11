package bot

import (
	"log"
	siikadb "siikabot/db"
	"siikabot/matrix"
	"strings"

	"github.com/matrix-org/gomatrix"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	db        *siikadb.DB
	client    matrix.Client
	adminUser string
)

func handleTextEvent(event *gomatrix.Event) {
	msgtype := ""
	if m, ok := event.Content["msgtype"].(string); ok {
		msgtype = m
	}
	metrics.eventsHandled.With(prometheus.Labels{"event_type": "m.room.message", "msg_type": msgtype}).Inc()
	if msgtype == "m.text" && event.Sender != client.UserID {
		msg := event.Content["body"].(string)
		msgCommand := strings.Split(msg, " ")[0]
		isCommand := true
		switch msgCommand {
		case "!ping":
			ping(event.RoomID, msg)
		case "!traceroute":
			traceroute(event.RoomID, msg)
		case "!ruuvi":
			ruuvi(event.RoomID, event.Sender, msg)
		case "!grafana":
			grafana(event.RoomID, event.Sender, msg)
		default:
			isCommand = false
		}
		if isCommand {
			metrics.commandsHandled.With(prometheus.Labels{"command": msgCommand}).Inc()
		}
	}
}

func handleMemberEvent(event *gomatrix.Event) {
	metrics.eventsHandled.With(prometheus.Labels{"event_type": "m.room.member", "msg_type": ""}).Inc()
	if event.Content["membership"] == "invite" && *event.StateKey == client.UserID {
		client.JoinRoom(event.RoomID)
		log.Print("Joined room " + event.RoomID)
	}
}

func Run(homeserverURL, userID, accessToken, hookSecret, dataPath, admin string) error {
	initMetrics()
	db = siikadb.NewDB(dataPath + "/siikabot.db")
	client = matrix.NewClient(homeserverURL, userID, accessToken)
	adminUser = admin

	client.OnEvent("m.room.member", handleMemberEvent)
	client.OnEvent("m.room.message", handleTextEvent)
	resp := client.InitialSync()
	for roomID, _ := range resp.Rooms.Invite {
		client.JoinRoom(roomID)
		log.Print("Joined room " + roomID)
	}
	initHTTP(hookSecret)
	return client.Sync()
}

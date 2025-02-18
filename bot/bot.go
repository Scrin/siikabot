package bot

import (
	"log"
	"strings"

	"github.com/Scrin/siikabot/db"
	"github.com/Scrin/siikabot/matrix"

	"github.com/matrix-org/gomatrix"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	adminUser string
)

func handleTextEvent(event *gomatrix.Event) {
	msgtype := ""
	if m, ok := event.Content["msgtype"].(string); ok {
		msgtype = m
	}
	metrics.eventsHandled.With(prometheus.Labels{"event_type": "m.room.message", "msg_type": msgtype}).Inc()
	if msgtype == "m.text" && event.Sender != matrix.GetUserID() {
		msg := event.Content["body"].(string)
		format, _ := event.Content["format"].(string)
		formattedBody, _ := event.Content["formatted_body"].(string)
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
		case "!remind":
			remind(event.RoomID, event.Sender, msg, format, formattedBody)
		case "!chat":
			chat(event.RoomID, event.Sender, msg)
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
	if event.Content["membership"] == "invite" && *event.StateKey == matrix.GetUserID() {
		matrix.JoinRoom(event.RoomID)
		log.Print("Joined room " + event.RoomID)
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

	matrix.OnEvent("m.room.member", handleMemberEvent)
	matrix.OnEvent("m.room.message", handleTextEvent)
	resp := matrix.InitialSync()
	for roomID := range resp.Rooms.Invite {
		matrix.JoinRoom(roomID)
		log.Print("Joined room " + roomID)
	}
	initReminder()
	initHTTP(hookSecret)
	openrouterAPIKey = openrouterApiKey
	return matrix.Sync()
}

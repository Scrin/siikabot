package bot

import (
	"log"
	siikadb "siikabot/db"
	"siikabot/matrix"
	"strings"

	"github.com/matrix-org/gomatrix"
)

var (
	db        *siikadb.DB
	client    matrix.Client
	adminUser string
)

func handleTextEvent(event *gomatrix.Event) {
	if event.Content["msgtype"] == "m.text" && event.Sender != client.UserID {
		msg := event.Content["body"].(string)
		switch strings.Split(msg, " ")[0] {
		case "!ping":
			ping(event.RoomID, msg)
		case "!traceroute":
			traceroute(event.RoomID, msg)
		case "!ruuvi":
			ruuvi(event.RoomID, event.Sender, msg)
		}
	}
}

func handleMemberEvent(event *gomatrix.Event) {
	if event.Content["membership"] == "invite" && *event.StateKey == client.UserID {
		client.JoinRoom(event.RoomID)
		log.Print("Joined room " + event.RoomID)
	}
}

func Run(homeserverURL, userID, accessToken, hookSecret, dataPath, admin string) error {
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

package bot

import (
	"log"
	"siikabot/matrix"
	"strings"

	"github.com/matrix-org/gomatrix"
)

var (
	client matrix.Client
)

func handleTextEvent(event *gomatrix.Event) {
	if event.Content["msgtype"] == "m.text" && event.Sender != client.UserID {
		if strings.HasPrefix(event.Content["body"].(string), "!ping") {
			ping(event.RoomID, event.Content["body"].(string))
		} else if strings.HasPrefix(event.Content["body"].(string), "!traceroute") {
			traceroute(event.RoomID, event.Content["body"].(string))
		}
	}
}

func handleMemberEvent(event *gomatrix.Event) {
	if event.Content["membership"] == "invite" && *event.StateKey == client.UserID {
		client.JoinRoom(event.RoomID)
		log.Print("Joined room " + event.RoomID)
	}
}

func Run(homeserverURL string, userID string, accessToken string) error {
	client = matrix.NewClient(homeserverURL, userID, accessToken)
	client.OnEvent("m.room.member", handleMemberEvent)
	client.OnEvent("m.room.message", handleTextEvent)
	resp := client.InitialSync()
	for roomID, _ := range resp.Rooms.Invite {
		client.JoinRoom(roomID)
		log.Print("Joined room " + roomID)
	}
	return client.Sync()
}

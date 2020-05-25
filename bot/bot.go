package bot

import (
	"log"
	"siikabot/matrix"
	"strings"

	"github.com/matrix-org/gomatrix"
)

type SiikaBot struct {
	client matrix.Client
}

func (bot SiikaBot) handleTextEvent(event *gomatrix.Event) {
	if event.Content["msgtype"] == "m.text" && event.Sender != bot.client.UserID {
		if strings.HasPrefix(event.Content["body"].(string), "!ping") {
			bot.ping(event.RoomID, event.Content["body"].(string))
		}
	}
}

func (bot SiikaBot) handleMemberEvent(event *gomatrix.Event) {
	if event.Content["membership"] == "invite" && *event.StateKey == bot.client.UserID {
		bot.client.JoinRoom(event.RoomID)
		log.Print("Joined room " + event.RoomID)
	}
}

func (bot SiikaBot) initialSync() {
	resp := bot.client.InitialSync()
	for roomID, _ := range resp.Rooms.Invite {
		bot.client.JoinRoom(roomID)
		log.Print("Joined room " + roomID)
	}
}

func (bot SiikaBot) Run() error {
	bot.initialSync()
	return bot.client.Sync()
}

func NewSiikaBot(homeserverURL string, userID string, accessToken string) SiikaBot {
	matrixClient := matrix.NewClient(homeserverURL, userID, accessToken)
	bot := SiikaBot{matrixClient}
	syncer := matrixClient.Syncer
	syncer.OnEventType("m.room.member", bot.handleMemberEvent)
	syncer.OnEventType("m.room.message", bot.handleTextEvent)
	return bot
}

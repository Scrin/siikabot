//go:build e2e

package e2e

import (
	"strings"
	"testing"

	"github.com/Scrin/siikabot/testutil/fakenet"
)

// The bot starts, joins rooms it's invited to, and answers a message that addresses it with what the model says
func TestBotAnswersWhenAddressed(t *testing.T) {
	e := setUp(t)
	alice := e.User(t, "alice")
	room := e.Room(t, alice, nil)

	asked := alice.Ask(t, room, "what is the capital of Finland?")
	if answer := room.WaitForBot(t, asked).Body; answer != fakenet.DefaultAnswer {
		t.Errorf("the bot answered %q, want the model's answer", answer)
	}
	requests := e.Model.Requests(room.ID.String())
	if len(requests) != 1 || !strings.Contains(requests[0].LastUserText(), "what is the capital of Finland?") {
		t.Fatalf("the model was asked %d times, last with %q", len(requests), requests[len(requests)-1].LastUserText())
	}
}

// An image message names its URL, and whoever sends it chooses it. The bot downloads only Matrix content URIs from
// its own homeserver, never anything else, which would get its access token (review finding S1).
func TestImageWithAForeignURLIsNeverFetched(t *testing.T) {
	e := setUp(t)
	mallory := e.User(t, "mallory")
	room := e.Room(t, mallory, nil)

	asked := mallory.Send(t, room, map[string]any{
		// With a filename of its own, the body is the image's caption, which addresses the bot
		"msgtype": "m.image", "body": botName + ": what is in this picture?", "filename": "steal.png",
		"url":  "https://attacker.example/steal",
		"info": map[string]any{"mimetype": "image/png"},
	})
	room.WaitForBot(t, asked)
	if requests := e.net.Requests("attacker.example"); len(requests) != 0 {
		t.Errorf("the bot requested the image's foreign URL: %+v", requests)
	}
}

//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/Scrin/siikabot/testutil/fakenet"
)

// What the dashboard shows, made through the bot the way people make it, for the tests of the API's contract with
// the frontend and of the dashboard in a browser

// dashboardWorld is a little of everything the dashboard shows: Alice and Bob in a named group room, and Alice's
// direct chat with the bot
type dashboardWorld struct {
	alice, bob *User
	room, dm   *Room
	roomName   string
	// Texts to look for: Alice's reminder, her memories in the group room and the direct chat, the bot's memory
	// of the group room, and the webhook's name
	reminder, memory, dmMemory, roomMemory, webhookName string
	webhookID                                           string
	// The answers of the calls that made things, which the contract test keeps: the pass over the room, and the
	// webhook as it was created and changed
	reflected, webhookCreated, webhookUpdated json.RawMessage
}

// newDashboardWorld makes the world. The admin makes the webhook, asks for the pass over the room and changes a
// setting, so a test that makes a world doesn't run in parallel.
func newDashboardWorld(t *testing.T, e *environment, admin *Dashboard) *dashboardWorld {
	t.Helper()
	w := &dashboardWorld{alice: e.User(t, "alice"), bob: e.User(t, "bob")}
	w.roomName = "Room " + randomString()[:8]
	w.room = e.Room(t, w.alice, []*User{w.bob}, WithName(w.roomName))
	w.dm = e.Room(t, w.alice, nil)
	w.reminder, w.memory, w.dmMemory = marker(t, "reminder"), marker(t, "memory"), marker(t, "dm-memory")
	w.roomMemory, w.webhookName = marker(t, "room-memory"), "Hook "+randomString()[:8]

	// Messages to count
	w.bob.Say(t, w.room, "a first message to count")
	w.bob.Say(t, w.room, "a second message to count")
	w.alice.Say(t, w.room, "and one of Alice's")

	w.room.WaitForBotMessage(t, w.alice.Say(t, w.room, "!remind 1h "+w.reminder), bodyContains(w.reminder))

	// A memory in each room, a turn that fails, and a pass that forms a memory of the group room
	e.Model.Script(t, w.room.ID.String(), func(req fakenet.ModelRequest) fakenet.Reply {
		switch {
		case req.Metadata["kind"] == "reflection":
			return fakenet.CallTool("pass-1", "update_room_memories",
				fmt.Sprintf(`{"changes":[{"action":"add","memory":"The room talks about %s"}]}`, w.roomMemory))
		case len(req.TurnToolResults()) > 0:
			return fakenet.Answer("saved")
		case strings.Contains(req.LastUserText(), "fail now"):
			return fakenet.Reply{Status: http.StatusBadGateway}
		default:
			return fakenet.CallTool("call-1", "memory", fmt.Sprintf(`{"action":"save","memory":%q}`, w.memory))
		}
	})
	w.room.WaitForBotMessage(t, w.alice.Ask(t, w.room, "remember this"), bodyContains("saved"))
	w.room.WaitForBot(t, w.alice.Ask(t, w.room, "fail now"))
	e.Model.Script(t, w.dm.ID.String(), toolThenAnswer("memory", fmt.Sprintf(`{"action":"save","memory":%q}`, w.dmMemory), "saved"))
	w.dm.WaitForBotMessage(t, w.alice.Ask(t, w.dm, "remember this too"), bodyContains("saved"))
	if code := admin.Call(t, http.MethodPost, "/api/admin/rooms/"+w.room.ID.String()+"/reflect", nil, &w.reflected); code != http.StatusOK {
		t.Fatalf("asking for a pass gave %d", code)
	}

	// A webhook that has posted, and been renamed
	token := randomString()
	if code := admin.Call(t, http.MethodPost, "/api/admin/webhooks",
		map[string]string{"name": w.webhookName, "room_id": w.room.ID.String(), "token": token}, &w.webhookCreated); code != http.StatusOK && code != http.StatusCreated {
		t.Fatalf("creating a webhook gave %d", code)
	}
	var created struct {
		Webhook struct {
			ID string `json:"id"`
		} `json:"webhook"`
	}
	if err := json.Unmarshal(w.webhookCreated, &created); err != nil {
		t.Fatal(err)
	}
	w.webhookID = created.Webhook.ID
	last := w.room.Latest(t)
	if code := e.PostHook(t, "/hooks/message/"+w.webhookID, []byte("The build passed"),
		map[string]string{"Authorization": "Bearer " + token}); code != http.StatusOK {
		t.Fatalf("posting through the webhook gave %d", code)
	}
	w.room.WaitForBotMessage(t, last, bodyContains("The build passed"))
	w.webhookName += " renamed"
	if code := admin.Call(t, http.MethodPatch, "/api/admin/webhooks/"+w.webhookID, map[string]string{"name": w.webhookName},
		&w.webhookUpdated); code != http.StatusOK {
		t.Fatalf("renaming the webhook gave %d", code)
	}

	// A configuration that changed something, for the configuration's history
	e.changeSettings(t, "max_response_tokens 3000")
	return w
}

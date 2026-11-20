//go:build e2e

package e2e

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// Commands, as people send them and as the room sees the bot's answers

func TestRemindDelivers(t *testing.T) {
	e := setUp(t)
	t.Parallel()
	alice := e.User(t, "alice")
	room := e.Room(t, alice, nil)
	text := marker(t, "remind")

	set := alice.Say(t, room, "!remind 2s "+text)
	if confirmed := room.WaitForBot(t, set); !strings.HasPrefix(confirmed.Body, "Reminding") {
		t.Fatalf("the bot answered %q, want the reminder confirmed", confirmed.Body)
	}
	room.WaitForBotMessage(t, set, func(content map[string]any) bool {
		body, _ := content["body"].(string)
		return strings.Contains(body, text) && !strings.HasPrefix(body, "Reminding")
	})
	if left := e.Query(t, "SELECT count(*) FROM reminders WHERE message LIKE '%' || $1 || '%'", text); left[0] != "0" {
		t.Errorf("%s reminders are left after delivery", left[0])
	}
}

// Deleting a reminder on the dashboard stops it going off
func TestReminderDeletedOnTheDashboardNeverGoesOff(t *testing.T) {
	e := setUp(t)
	t.Parallel()
	alice := e.User(t, "alice")
	room := e.Room(t, alice, nil)
	dashboard := e.Login(t, alice, room)
	text := marker(t, "deleted")

	set := alice.Say(t, room, "!remind 4s "+text)
	room.WaitForBot(t, set)
	var reminders struct {
		Reminders []struct {
			ID int64 `json:"id"`
		} `json:"reminders"`
	}
	if code := dashboard.Call(t, http.MethodGet, "/api/reminders", nil, &reminders); code != http.StatusOK || len(reminders.Reminders) != 1 {
		t.Fatalf("listing the reminders gave %d, %+v", code, reminders)
	}
	if code := dashboard.Call(t, http.MethodDelete, "/api/reminders/"+itoa(reminders.Reminders[0].ID), nil, nil); code != http.StatusOK {
		t.Fatalf("deleting the reminder gave %d", code)
	}

	time.Sleep(6 * time.Second)
	for _, evt := range room.botMessagesAfter(t, set) {
		if body, _ := evt.Content.Raw["body"].(string); strings.Contains(body, text) && !strings.HasPrefix(body, "Reminding") {
			t.Errorf("the deleted reminder went off: %q", body)
		}
	}
}

// Only the admin changes the chat configuration, which is kept as a history of rows
func TestChatSettings(t *testing.T) {
	e := setUp(t)
	mallory := e.User(t, "mallory")
	room := e.Room(t, e.admin, []*User{mallory})

	if refused := room.WaitForBot(t, mallory.Say(t, room, "!chat set max_tool_iterations 3")); !strings.Contains(refused.Body, "Only the admin") {
		t.Errorf("someone else changing a setting got %q", refused.Body)
	}
	before := e.Query(t, "SELECT count(*) FROM chat_config")[0]
	changed := room.WaitForBot(t, e.admin.Say(t, room, "!chat set max_tool_iterations 7"))
	if after := e.Query(t, "SELECT count(*) FROM chat_config")[0]; after == before {
		t.Fatalf("the admin's change added no configuration row (still %s); the bot said %q", after, changed.Body)
	}
	if latest := e.Query(t, "SELECT max_tool_iterations FROM chat_config ORDER BY id DESC LIMIT 1")[0]; latest != "7" {
		t.Errorf("max_tool_iterations is %s, want 7", latest)
	}
	if listed := room.WaitForBot(t, e.admin.Say(t, room, "!chat config")); !strings.Contains(listed.Body, "max_tool_iterations") {
		t.Errorf("!chat config listed %q", listed.Body)
	}
}

// Logging in to the dashboard with !auth, and the ways it must not be abused (review findings S5 and B12)
func TestDashboardLogin(t *testing.T) {
	e := setUp(t)
	t.Parallel()
	alice, mallory := e.User(t, "alice"), e.User(t, "mallory")
	room := e.Room(t, alice, []*User{mallory})

	dashboard := e.Login(t, alice, room)
	var me struct {
		UserID string `json:"user_id"`
	}
	if code := dashboard.Call(t, http.MethodGet, "/api/auth/me", nil, &me); code != http.StatusOK || me.UserID != alice.ID.String() {
		t.Fatalf("/api/auth/me gave %d, %+v", code, me)
	}

	// Mallory sees Alice's challenge in the room, and sends it again
	var challenge struct {
		Challenge  string `json:"challenge"`
		PollSecret string `json:"poll_secret"`
	}
	e.api(t, http.MethodPost, "/api/auth/challenge", "", nil, &challenge)
	room.WaitForBot(t, alice.Say(t, room, "!auth "+challenge.Challenge))
	if again := room.WaitForBot(t, mallory.Say(t, room, "!auth "+challenge.Challenge)); !strings.Contains(again.Body, "already been used") {
		t.Errorf("completing Alice's challenge again got %q", again.Body)
	}
	var poll struct {
		UserID string `json:"user_id"`
	}
	e.api(t, http.MethodPost, "/api/auth/poll", "", map[string]string{"challenge": challenge.Challenge, "poll_secret": challenge.PollSecret}, &poll)
	if poll.UserID != alice.ID.String() {
		t.Errorf("Alice's browser was logged in as %q", poll.UserID)
	}

	// A mistyped challenge leaves the session Alice has alone
	second := e.Login(t, alice, room)
	room.WaitForBot(t, alice.Say(t, room, "!auth not-a-challenge"))
	if code := second.Call(t, http.MethodGet, "/api/auth/me", nil, nil); code != http.StatusOK {
		t.Errorf("after a mistyped !auth, Alice's session gets %d", code)
	}

	room.WaitForBot(t, alice.Say(t, room, "!auth logout"))
	if code := second.Call(t, http.MethodGet, "/api/auth/me", nil, nil); code != http.StatusUnauthorized {
		t.Errorf("after !auth logout, Alice's session gets %d, want 401", code)
	}
}

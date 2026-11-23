//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/Scrin/siikabot/testutil/fakenet"
)

// The commands and tools not covered elsewhere, and every endpoint of the dashboard's API against real data

func TestPing(t *testing.T) {
	e := setUp(t)
	t.Parallel()
	alice := e.User(t, "alice")
	room := e.Room(t, alice, nil)

	answer := room.WaitForBot(t, alice.Say(t, room, "!ping 127.0.0.1"))
	if !strings.Contains(answer.Body, "127.0.0.1") || !strings.Contains(answer.Body, "packet") {
		t.Errorf("!ping answered %q", answer.Body)
	}
	// A missing target is a usage error, not a crash (B22)
	t.Run("ipv6_without_a_target", func(t *testing.T) {
		knownBug(t, "B22", "!ping -6 without a target panics, and the room gets no answer")
		room.WaitForBot(t, alice.Say(t, room, "!ping -6"))
	})
}

func TestTraceroute(t *testing.T) {
	e := setUp(t)
	t.Parallel()
	alice := e.User(t, "alice")
	room := e.Room(t, alice, nil)

	if answer := room.WaitForBot(t, alice.Say(t, room, "!traceroute 127.0.0.1")); !strings.Contains(answer.Body, "127.0.0.1") {
		t.Errorf("!traceroute answered %q", answer.Body)
	}
}

func TestStats(t *testing.T) {
	e := setUp(t)
	t.Parallel()
	alice, bob := e.User(t, "alice"), e.User(t, "bob")
	room := e.Room(t, alice, []*User{bob})
	for i := range 3 {
		bob.Say(t, room, fmt.Sprintf("message number %d", i))
	}

	// !stats is a message too, which may be counted by the time the bot answers
	own := room.WaitForBot(t, bob.Say(t, room, "!stats"))
	if !strings.Contains(own.Body, "Messages: 3") && !strings.Contains(own.Body, "Messages: 4") {
		t.Errorf("Bob's !stats answered %q", own.Body)
	}
	if never := room.WaitForBot(t, alice.Say(t, room, "!stats @nobody:localhost")); !strings.Contains(never.Body, "No stats") {
		t.Errorf("!stats for someone with no messages answered %q", never.Body)
	}
}

// The test homeserver doesn't federate, so !servers can only report it as failing; it must still answer
func TestServers(t *testing.T) {
	e := setUp(t)
	t.Parallel()
	alice := e.User(t, "alice")
	room := e.Room(t, alice, nil)

	if answer := room.WaitForBot(t, alice.Say(t, room, "!servers")); !strings.Contains(answer.Body, "localhost") {
		t.Errorf("!servers answered %q", answer.Body)
	}
}

// The configuration's history lists every change, and reverting appends a copy of an earlier row
func TestChatConfigHistoryAndRevert(t *testing.T) {
	e := setUp(t)
	room := e.Room(t, e.admin, nil)
	original := e.Query(t, "SELECT max(id) FROM chat_config")[0]
	e.changeSettings(t, "max_response_tokens 3000")

	if history := room.WaitForBot(t, e.admin.Say(t, room, "!chat config history")); !strings.Contains(history.Body, "max_response_tokens") {
		t.Errorf("the history lists %q", history.Body)
	}
	room.WaitForBot(t, e.admin.Say(t, room, "!chat config revert "+original))
	if tokens := e.Query(t, "SELECT max_response_tokens FROM chat_config ORDER BY id DESC LIMIT 1")[0]; tokens == "3000" {
		t.Error("reverting left the change in place")
	}
	if rows := e.Query(t, "SELECT count(*) FROM chat_config WHERE id > $1::bigint", original)[0]; rows != "2" {
		t.Errorf("%s rows were added, want the change and the revert's copy", rows)
	}
}

// The reminder tool sets a reminder, and deleting one posts the command that sets it again
func TestManageRemindersCreatesAndDeletes(t *testing.T) {
	e := setUp(t)
	t.Parallel()
	alice := e.User(t, "alice")
	room := e.Room(t, alice, nil)
	text := marker(t, "tool")

	e.Model.Script(t, room.ID.String(), toolThenAnswer("manage_reminders",
		fmt.Sprintf(`{"action":"create","time":"1h","message":%q}`, text), "set"))
	room.WaitForBotMessage(t, alice.Ask(t, room, "remind me"), bodyContains("[AI tool call] Reminding"))
	ids := e.Query(t, "SELECT id FROM reminders WHERE message = $1", text)
	if len(ids) != 1 {
		t.Fatalf("%d reminders were stored, want the one the tool set", len(ids))
	}

	e.Model.Script(t, room.ID.String(), toolThenAnswer("manage_reminders",
		fmt.Sprintf(`{"action":"delete","reminder_id":%s}`, ids[0]), "deleted"))
	// toolThenAnswer looks at the turn's own tool results, not the first turn's
	notice := room.WaitForBotMessage(t, alice.Ask(t, room, "never mind"), bodyContains("[AI tool call] Deleted the reminder"))
	if body, _ := notice["body"].(string); !strings.Contains(body, "!remind ") || !strings.Contains(body, text) {
		t.Errorf("the deletion notice doesn't give the command that sets it again: %q", body)
	}
	if left := e.Query(t, "SELECT count(*) FROM reminders WHERE message = $1", text)[0]; left != "0" {
		t.Error("the reminder is still there")
	}
}

// The message stats tool counts only the turn's room
func TestMessageStatsToolStaysInTheRoom(t *testing.T) {
	e := setUp(t)
	t.Parallel()
	alice, bob := e.User(t, "alice"), e.User(t, "bob")
	room := e.Room(t, alice, []*User{bob})
	elsewhere := e.Room(t, bob, nil)
	for i := range 7 {
		bob.Say(t, elsewhere, fmt.Sprintf("elsewhere %d", i))
	}
	bob.Say(t, room, "here")

	e.Model.Script(t, room.ID.String(), toolThenAnswer("get_message_stats",
		fmt.Sprintf(`{"kind":"users","user_ids":[%q]}`, bob.ID), "done"))
	room.WaitForBot(t, alice.Ask(t, room, "how much does Bob talk?"))
	var result string
	for _, req := range e.Model.Requests(room.ID.String()) {
		if r, ok := req.ToolResults()["call-1"]; ok {
			result = r
		}
	}
	if !strings.Contains(result, "1 message") || strings.Contains(result, "7 messages") || strings.Contains(result, "8 messages") {
		t.Errorf("Bob's stats in the room were:\n%s", result)
	}
}

// containsText reports whether a JSON answer holds text anywhere in it
func containsText(v any, text string) bool {
	data, _ := json.Marshal(v)
	return strings.Contains(string(data), text)
}

// findID finds the id of the object in a JSON answer whose field holds text
func findID(v any, field, text string) (int64, bool) {
	switch v := v.(type) {
	case map[string]any:
		if s, ok := v[field].(string); ok && strings.Contains(s, text) {
			if id, ok := v["id"].(float64); ok {
				return int64(id), true
			}
		}
		for _, child := range v {
			if id, ok := findID(child, field, text); ok {
				return id, true
			}
		}
	case []any:
		for _, child := range v {
			if id, ok := findID(child, field, text); ok {
				return id, true
			}
		}
	}
	return 0, false
}

// Every endpoint people use answers with what the bot holds for them
func TestPeoplesAPI(t *testing.T) {
	e := setUp(t)
	t.Parallel()
	alice, bob := e.User(t, "alice"), e.User(t, "bob")
	room := e.Room(t, alice, []*User{bob})
	dashboard := e.Login(t, alice, room)
	reminder, memory := marker(t, "reminder"), marker(t, "memory")

	room.WaitForBot(t, alice.Say(t, room, "!remind 1h "+reminder))
	e.Model.Script(t, room.ID.String(), toolThenAnswer("memory", fmt.Sprintf(`{"action":"save","memory":%q}`, memory), "saved"))
	room.WaitForBot(t, alice.Ask(t, room, "remember this"))
	bob.Say(t, room, "a message to count")

	get := func(path string) any {
		t.Helper()
		var answer any
		if code := dashboard.Call(t, http.MethodGet, path, nil, &answer); code != http.StatusOK {
			t.Fatalf("GET %s gave %d", path, code)
		}
		return answer
	}
	if !containsText(get("/api/reminders"), reminder) {
		t.Error("/api/reminders doesn't list the reminder")
	}
	if !containsText(get("/api/rooms"), room.ID.String()) {
		t.Error("/api/rooms doesn't list the room")
	}
	if members := get("/api/rooms/" + room.ID.String() + "/members"); !containsText(members, alice.ID.String()) || !containsText(members, bob.ID.String()) {
		t.Errorf("the room's members are %v", members)
	}
	// The bot counts a message once it has handled it
	eventually(t, "the room's stats to count Bob", func() bool {
		return containsText(get("/api/rooms/"+room.ID.String()+"/stats"), bob.ID.String())
	})

	memories := get("/api/memories")
	id, ok := findID(memories, "memory", memory)
	if !ok {
		t.Fatalf("/api/memories doesn't list the memory: %v", memories)
	}
	if code := dashboard.Call(t, http.MethodDelete, fmt.Sprintf("/api/memories/%d", id), nil, nil); code != http.StatusOK {
		t.Errorf("deleting the memory gave %d", code)
	}
	if containsText(get("/api/memories"), memory) {
		t.Error("the deleted memory is still listed")
	}
	var cleared struct {
		DeletedCount int `json:"deleted_count"`
	}
	if code := dashboard.Call(t, http.MethodDelete, "/api/memories", nil, &cleared); code != http.StatusOK {
		t.Errorf("clearing the memories gave %d", code)
	}

	for _, path := range []string{"/api/healthcheck", "/api/metrics"} {
		if code := e.api(t, http.MethodGet, path, "", nil, nil); code != http.StatusOK {
			t.Errorf("GET %s gave %d", path, code)
		}
	}
}

// Every admin endpoint answers with what the bot holds, and the deletes take effect
func TestAdminAPI(t *testing.T) {
	e := setUp(t)
	alice, bob := e.User(t, "alice"), e.User(t, "bob")
	room := e.Room(t, alice, []*User{bob})
	admin := e.Login(t, e.admin, e.Room(t, e.admin, nil))
	memory := marker(t, "room-memory")

	reflection(t, e, room, func(fakenet.ModelRequest) fakenet.Reply {
		return fakenet.CallTool("pass-1", "update_room_memories",
			fmt.Sprintf(`{"changes":[{"action":"add","memory":"The room likes %s"}]}`, memory))
	})
	room.WaitForBot(t, alice.Ask(t, room, "let's talk"))
	bob.Say(t, room, "counted")
	reflect(t, admin, room)

	get := func(path string) any {
		t.Helper()
		var answer any
		if code := admin.Call(t, http.MethodGet, path, nil, &answer); code != http.StatusOK {
			t.Fatalf("GET %s gave %d", path, code)
		}
		return answer
	}
	roomID := room.ID.String()
	// The bot counts a message once it has handled it
	eventually(t, "the room's stats to count Bob", func() bool {
		return containsText(get("/api/admin/rooms/"+roomID+"/stats"), bob.ID.String())
	})
	for path, want := range map[string]string{
		"/api/admin/rooms":                        roomID,
		"/api/admin/rooms/" + roomID + "/members": bob.ID.String(),
		"/api/admin/rooms/" + roomID + "/stats":   bob.ID.String(),
		"/api/admin/chat-usage":                   roomID,
		"/api/admin/message-activity":             roomID,
		"/api/admin/chat-config":                  "max_tool_iterations",
		"/api/admin/room-memories":                memory,
	} {
		if answer := get(path); !containsText(answer, want) {
			t.Errorf("GET %s lacks %q: %v", path, want, answer)
		}
	}

	id, ok := findID(get("/api/admin/room-memories"), "memory", memory)
	if !ok {
		t.Fatal("the room memory has no id")
	}
	if code := admin.Call(t, http.MethodDelete, fmt.Sprintf("/api/admin/room-memories/%d", id), nil, nil); code != http.StatusOK {
		t.Errorf("deleting the room memory gave %d", code)
	}
	if code := admin.Call(t, http.MethodDelete, fmt.Sprintf("/api/admin/room-memories/%d", id), nil, nil); code != http.StatusNotFound {
		t.Errorf("deleting it again gave %d, want 404", code)
	}
	if code := admin.Call(t, http.MethodDelete, "/api/admin/rooms/"+roomID+"/memories", nil, nil); code != http.StatusOK {
		t.Errorf("deleting the room's memories gave %d", code)
	}
	if left := e.Query(t, "SELECT count(*) FROM room_memory WHERE room_id = $1", roomID)[0]; left != "0" {
		t.Errorf("%s room memories are left", left)
	}
}

// The admin manages webhooks: creates, renames, moves and deletes them, and the name has to be free
func TestWebhookManagement(t *testing.T) {
	e := setUp(t)
	room := e.Room(t, e.admin, nil)
	other := e.Room(t, e.admin, nil)
	admin := e.Login(t, e.admin, room)
	name := "Hook " + randomString()[:8]

	var created struct {
		Webhook struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"webhook"`
	}
	body := map[string]string{"name": name, "room_id": room.ID.String(), "token": randomString()}
	if code := admin.Call(t, http.MethodPost, "/api/admin/webhooks", body, &created); code != http.StatusOK && code != http.StatusCreated {
		t.Fatalf("creating a webhook gave %d", code)
	}
	if code := admin.Call(t, http.MethodPost, "/api/admin/webhooks", body, nil); code != http.StatusConflict {
		t.Errorf("a second webhook with the same name gave %d, want 409", code)
	}
	path := "/api/admin/webhooks/" + created.Webhook.ID
	if code := admin.Call(t, http.MethodPatch, path, map[string]string{"room_id": other.ID.String()}, nil); code != http.StatusOK {
		t.Errorf("moving the webhook gave %d", code)
	}
	var list any
	admin.Call(t, http.MethodGet, "/api/admin/webhooks", nil, &list)
	if !containsText(list, other.ID.String()) {
		t.Errorf("the webhook list doesn't show the move: %v", list)
	}
	if code := admin.Call(t, http.MethodDelete, path, nil, nil); code != http.StatusOK && code != http.StatusNoContent {
		t.Errorf("deleting the webhook gave %d", code)
	}
	if code := e.PostHook(t, "/hooks/message/"+created.Webhook.ID, []byte("x"),
		map[string]string{"Authorization": "Bearer " + body["token"]}); code == http.StatusOK {
		t.Error("a deleted webhook still posts")
	}
}

// Logging out ends the session: its token no longer works, anywhere in the API
func TestLogout(t *testing.T) {
	e := setUp(t)
	t.Parallel()
	alice := e.User(t, "alice")
	room := e.Room(t, alice, nil)
	dashboard := e.Login(t, alice, room)
	if code := dashboard.Call(t, http.MethodGet, "/api/auth/me", nil, nil); code != http.StatusOK {
		t.Fatalf("before logging out, /api/auth/me gave %d", code)
	}

	if code := dashboard.Call(t, http.MethodPost, "/api/auth/logout", nil, nil); code != http.StatusOK {
		t.Fatalf("logging out gave %d", code)
	}
	for _, path := range []string{"/api/auth/me", "/api/reminders", "/api/rooms", "/api/memories"} {
		if code := dashboard.Call(t, http.MethodGet, path, nil, nil); code != http.StatusUnauthorized {
			t.Errorf("after logging out, %s gave %d", path, code)
		}
	}
	// Logging out twice is no error, since the session is gone either way
	if code := dashboard.Call(t, http.MethodPost, "/api/auth/logout", nil, nil); code != http.StatusOK {
		t.Errorf("logging out again gave %d", code)
	}
}

// !chat config <key> describes one setting with its history, for anyone, and suggests a key for one it doesn't know
func TestChatConfigShowsASetting(t *testing.T) {
	e := setUp(t)
	t.Parallel()
	alice := e.User(t, "alice")
	room := e.Room(t, alice, nil)

	described := room.WaitForBot(t, alice.Say(t, room, "!chat config max_response_tokens"))
	if !strings.Contains(described.Body, "max_response_tokens") {
		t.Errorf("!chat config max_response_tokens answered %q", described.Body)
	}
	if asked := room.WaitForBot(t, alice.Say(t, room, "!chat set max_response_tokens")); asked.Body != described.Body {
		t.Errorf("!chat set with no value answered %q, not what !chat config does", asked.Body)
	}
	// A part of a key is enough to be pointed at it, and anything else lists the keys there are
	if partial := room.WaitForBot(t, alice.Say(t, room, "!chat config response_tokens")); !strings.Contains(partial.Body, "Did you mean max_response_tokens") {
		t.Errorf("a part of a key answered %q", partial.Body)
	}
	if unknown := room.WaitForBot(t, alice.Say(t, room, "!chat config nonsense")); !strings.Contains(unknown.Body, "room_memory_delay") {
		t.Errorf("an unknown key answered %q", unknown.Body)
	}
	if limited := room.WaitForBot(t, alice.Say(t, room, "!chat config history 1")); strings.Count(limited.Body, " - #") != 1 {
		t.Errorf("!chat config history 1 answered %q", limited.Body)
	}
	if bad := room.WaitForBot(t, alice.Say(t, room, "!chat config history many")); bad.Body == "" {
		t.Error("!chat config history with a count that isn't a number gave no answer")
	}
}

// The FMI tool as deployed: its requests reach FMI's hosts, and what it makes of the answers reaches the model,
// with FMI credited. A kind it doesn't know is an error the model is told about.
func TestFMITool(t *testing.T) {
	e := setUp(t)
	t.Parallel()
	alice := e.User(t, "alice")
	room := e.Room(t, alice, nil)
	warnings, err := os.ReadFile("../fmi/testdata/warnings.xml")
	if err != nil {
		t.Fatal(err)
	}
	e.net.Handle(t, "opendata.fmi.fi", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("producer") {
		case "pal_skandinavia":
			_, _ = io.WriteString(w, `[{"name":"Helsinki","region":"Helsinki","lat":60.16952,"lon":24.93545}]`)
		case "opendata":
			_, _ = io.WriteString(w, `[{"stationname":"Helsinki Kaisaniemi","distance":0.8,"utctime":"20261009T194000",`+
				`"temperature":9.9,"windspeedms":1.9,"winddirection":265.0,"humidity":87.0}]`)
		default:
			http.Error(w, "Unknown producer", http.StatusBadRequest)
		}
	}))
	e.net.Handle(t, "alerts.fmi.fi", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/atom+xml")
		_, _ = w.Write(warnings)
	}))
	call := func(id, arguments string) fakenet.ToolCall {
		c := fakenet.ToolCall{ID: id, Type: "function"}
		c.Function.Name, c.Function.Arguments = "get_fmi_data", arguments
		return c
	}
	e.Model.Script(t, room.ID.String(), func(req fakenet.ModelRequest) fakenet.Reply {
		if len(req.TurnToolResults()) == 0 {
			return fakenet.Reply{FinishReason: "tool_calls", ToolCalls: []fakenet.ToolCall{
				call("call-1", `{"kind":"current","location":"Helsinki"}`),
				call("call-2", `{"kind":"warnings"}`),
				call("call-3", `{"kind":"tides","location":"Helsinki"}`),
			}}
		}
		return fakenet.Answer("here is the weather report")
	})
	room.WaitForBotMessage(t, alice.Ask(t, room, "what's the weather?"), bodyContains("here is the weather report"))

	requests := e.Model.Requests(room.ID.String())
	results := requests[len(requests)-1].TurnToolResults()
	const credit = "Finnish Meteorological Institute"
	if current := results["call-1"]; !strings.Contains(current, "Helsinki Kaisaniemi") || !strings.Contains(current, "9.9") ||
		!strings.Contains(current, credit) {
		t.Errorf("the current weather reached the model as %q", current)
	}
	if active := results["call-2"]; !strings.Contains(active, "wind warning for land areas") || !strings.Contains(active, credit) {
		t.Errorf("the warnings reached the model as %q", active)
	}
	if unknown := results["call-3"]; !strings.Contains(unknown, "Error executing get_fmi_data") {
		t.Errorf("a kind the tool doesn't know reached the model as %q", unknown)
	}
}

// The API refuses requests it can't act on with a client error that says why: periods it doesn't keep, IDs that
// aren't numbers, webhooks that fail its rules or aren't there. It logs the admin in, so it doesn't run in parallel.
func TestAPIRejectsBadRequests(t *testing.T) {
	e := setUp(t)
	alice := e.User(t, "alice")
	room := e.Room(t, alice, nil)
	people := e.Login(t, alice, room)
	admin := e.Login(t, e.admin, e.Room(t, e.admin, nil))
	stats := "/api/rooms/" + room.ID.String() + "/stats"

	for _, call := range []struct {
		who          *Dashboard
		method, path string
		body         any
		want         int
	}{
		{people, http.MethodGet, stats + "?days=0", nil, http.StatusBadRequest},
		{people, http.MethodGet, stats + "?days=367", nil, http.StatusBadRequest},
		{people, http.MethodGet, stats + "?days=week", nil, http.StatusBadRequest},
		{people, http.MethodGet, stats + "?days=366", nil, http.StatusOK},
		{admin, http.MethodGet, "/api/admin/rooms/" + room.ID.String() + "/stats?days=-1", nil, http.StatusBadRequest},
		{admin, http.MethodGet, "/api/admin/message-activity?days=91", nil, http.StatusBadRequest},
		{admin, http.MethodGet, "/api/admin/message-activity?days=90", nil, http.StatusOK},
		{admin, http.MethodGet, "/api/admin/chat-usage?days=91", nil, http.StatusBadRequest},
		{admin, http.MethodGet, "/api/admin/chat-usage?days=x", nil, http.StatusBadRequest},
		{admin, http.MethodGet, "/api/admin/chat-usage?days=30", nil, http.StatusOK},
		{people, http.MethodDelete, "/api/memories/first", nil, http.StatusBadRequest},
		{people, http.MethodDelete, "/api/reminders/next", nil, http.StatusBadRequest},
		{admin, http.MethodDelete, "/api/admin/room-memories/all", nil, http.StatusBadRequest},
		{admin, http.MethodPost, "/api/admin/webhooks", "not an object", http.StatusBadRequest},
		{admin, http.MethodPost, "/api/admin/webhooks", map[string]string{"name": "", "room_id": room.ID.String()}, http.StatusBadRequest},
		{admin, http.MethodPost, "/api/admin/webhooks", map[string]string{"name": "fine", "room_id": "not a room"}, http.StatusBadRequest},
		{admin, http.MethodPatch, "/api/admin/webhooks/missing", map[string]string{"room_id": room.ID.String()}, http.StatusNotFound},
		{admin, http.MethodPatch, "/api/admin/webhooks/missing", "not an object", http.StatusBadRequest},
		{admin, http.MethodDelete, "/api/admin/webhooks/missing", nil, http.StatusNotFound},
	} {
		var answer map[string]any
		code := call.who.Call(t, call.method, call.path, call.body, &answer)
		if code != call.want {
			t.Errorf("%s %s gave %d, want %d", call.method, call.path, code, call.want)
		}
		if code >= 400 && code < 500 {
			if message, _ := answer["error"].(string); message == "" {
				t.Errorf("%s %s gave %d with no error to show", call.method, call.path, code)
			}
		}
	}
}

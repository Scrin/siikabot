//go:build e2e

package e2e

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Scrin/siikabot/testutil/fakenet"
)

// The chat turn and the bot's memories of rooms, through the scripted model

// toolThenAnswer calls a tool once, then answers with the given text
func toolThenAnswer(tool, arguments, answer string) fakenet.Script {
	return func(req fakenet.ModelRequest) fakenet.Reply {
		if len(req.TurnToolResults()) == 0 {
			return fakenet.CallTool("call-1", tool, arguments)
		}
		return fakenet.Answer(answer)
	}
}

// changeSettings has the admin change chat settings for the rest of a test, and puts them back after it. The
// settings are global, so a test that changes them doesn't run in parallel.
func (e *environment) changeSettings(t *testing.T, changes ...string) {
	t.Helper()
	room := e.Room(t, e.admin, nil)
	original := e.Query(t, "SELECT max(id) FROM chat_config")[0]
	for _, change := range changes {
		before := e.Query(t, "SELECT max(id) FROM chat_config")[0]
		answer := room.WaitForBot(t, e.admin.Say(t, room, "!chat set "+change))
		if e.Query(t, "SELECT max(id) FROM chat_config")[0] == before {
			t.Fatalf("changing %s added no configuration; the bot said %q", change, answer.Body)
		}
	}
	t.Cleanup(func() {
		room.WaitForBot(t, e.admin.Say(t, room, "!chat config revert "+original))
	})
}

// A tool call goes out, its result comes back to the model, and both are kept with the turn
func TestToolRoundTrip(t *testing.T) {
	e := setUp(t)
	t.Parallel()
	alice := e.User(t, "alice")
	room := e.Room(t, alice, nil)
	e.Model.Script(t, room.ID.String(), toolThenAnswer("get_time", `{"timezone":"Europe/Helsinki"}`, "It's noon"))

	if answer := room.WaitForBot(t, alice.Ask(t, room, "what time is it?")); answer.Body != "It's noon" && answer.Body != "It’s noon" {
		t.Errorf("the bot answered %q", answer.Body)
	}
	requests := e.Model.Requests(room.ID.String())
	if len(requests) != 2 || !strings.Contains(requests[1].ToolResults()["call-1"], "Helsinki") {
		t.Fatalf("the model got %d requests, the tool result %q", len(requests), requests[len(requests)-1].ToolResults()["call-1"])
	}
	if rows := e.Query(t, "SELECT count(*) FROM chat_history WHERE room_id = $1 AND message_type IN ('tool_call', 'tool_response')",
		room.ID.String()); rows[0] != "2" {
		t.Errorf("%s tool rows are stored, want the call and its result", rows[0])
	}
}

// A turn with one round of tool calls is recorded as one (B7)
func TestToolRoundsAreCounted(t *testing.T) {
	knownBug(t, "B7", "tool_iterations is off by one")
	e := setUp(t)
	t.Parallel()
	alice := e.User(t, "alice")
	room := e.Room(t, alice, nil)
	e.Model.Script(t, room.ID.String(), toolThenAnswer("get_time", `{"timezone":"Europe/Helsinki"}`, "noon"))

	room.WaitForBot(t, alice.Ask(t, room, "what time is it?"))
	eventually(t, "the turn's usage", func() bool {
		return len(e.Query(t, "SELECT tool_iterations FROM chat_usage WHERE room_id = $1", room.ID.String())) == 1
	})
	if rounds := e.Query(t, "SELECT tool_iterations FROM chat_usage WHERE room_id = $1", room.ID.String())[0]; rounds != "1" {
		t.Errorf("one round of tool calls is recorded as %s", rounds)
	}
}

// Past the tool limit, the model is asked for an answer without calling tools
func TestToolLimit(t *testing.T) {
	e := setUp(t)
	e.changeSettings(t, "max_tool_iterations 1")
	alice := e.User(t, "alice")
	room := e.Room(t, alice, nil)
	calls := 0
	e.Model.Script(t, room.ID.String(), func(req fakenet.ModelRequest) fakenet.Reply {
		if !req.ToolsAllowed() {
			return fakenet.Answer("final answer")
		}
		calls++
		return fakenet.CallTool(fmt.Sprintf("call-%d", calls), "get_time", `{"timezone":"UTC"}`)
	})

	if answer := room.WaitForBot(t, alice.Ask(t, room, "keep calling tools")); answer.Body != "final answer" {
		t.Errorf("past the tool limit the bot answered %q", answer.Body)
	}
}

// A model that keeps failing gets the room a failure message, not silence
func TestModelFailure(t *testing.T) {
	e := setUp(t)
	t.Parallel()
	alice := e.User(t, "alice")
	room := e.Room(t, alice, nil)
	e.Model.Script(t, room.ID.String(), func(fakenet.ModelRequest) fakenet.Reply { return fakenet.Reply{Status: http.StatusBadGateway} })

	answer := room.WaitForBot(t, alice.Ask(t, room, "hello?"))
	if answer.Body == fakenet.DefaultAnswer || answer.Body == "" {
		t.Errorf("with the model failing the bot said %q", answer.Body)
	}
	eventually(t, "the turn's usage", func() bool {
		return len(e.Query(t, "SELECT outcome FROM chat_usage WHERE room_id = $1", room.ID.String())) == 1
	})
	if outcome := e.Query(t, "SELECT outcome FROM chat_usage WHERE room_id = $1", room.ID.String())[0]; outcome == "ok" {
		t.Error("the failed turn is recorded as ok")
	}
}

// A model call that takes longer than model_timeout is given up on, and the room is told
func TestModelTimeout(t *testing.T) {
	e := setUp(t)
	e.changeSettings(t, "model_attempts 1", "model_timeout 10s", "turn_timeout 30s")
	alice := e.User(t, "alice")
	room := e.Room(t, alice, nil)
	e.Model.Script(t, room.ID.String(), func(fakenet.ModelRequest) fakenet.Reply {
		return fakenet.Reply{Text: "too late", Delay: 20 * time.Second}
	})

	started := time.Now()
	answer := room.WaitForBot(t, alice.Ask(t, room, "take your time"))
	if answer.Body == "too late" {
		t.Error("the bot waited for the slow model")
	}
	if took := time.Since(started); took > 19*time.Second {
		t.Errorf("the bot took %v to give up, past the model's delay", took)
	}
}

// A message that only replies to the bot may need no answer, and the model can say so
func TestNoReply(t *testing.T) {
	e := setUp(t)
	t.Parallel()
	alice := e.User(t, "alice")
	room := e.Room(t, alice, nil)
	e.Model.Script(t, room.ID.String(), func(req fakenet.ModelRequest) fakenet.Reply {
		if strings.Contains(req.LastUserText(), "thanks") {
			return fakenet.Answer("NO_REPLY")
		}
		return fakenet.Answer(fakenet.DefaultAnswer)
	})

	answer := room.WaitForBot(t, alice.Ask(t, room, "what's two and two?"))
	thanks := alice.Reply(t, room, answer.ID, "thanks")
	eventually(t, "the model to be asked about the reply", func() bool { return len(e.Model.Requests(room.ID.String())) == 2 })
	room.ExpectSilence(t, thanks, 2*time.Second)
}

// !chat reset during a turn leaves nothing of the turn behind (B3)
func TestResetDuringATurn(t *testing.T) {
	knownBug(t, "B3", "!chat reset doesn't wait for a turn already running")
	e := setUp(t)
	t.Parallel()
	alice := e.User(t, "alice")
	room := e.Room(t, alice, nil)
	release := heldModel(t, e, room)
	secret := marker(t, "secret")

	asked := alice.Ask(t, room, secret)
	waitForTheTurn(t, e, room)
	room.WaitForBotMessage(t, alice.Say(t, room, "!chat reset"), bodyContains("reset"))
	release()
	room.WaitForBotMessage(t, asked, func(content map[string]any) bool {
		body, _ := content["body"].(string)
		return strings.TrimSpace(body) == fakenet.DefaultAnswer
	})
	if historyHolds(t, e, room, secret) {
		t.Error("a turn that was running during !chat reset is stored after it")
	}
}

// reflect has the admin ask for a pass over a room on the dashboard
func reflect(t *testing.T, admin *Dashboard, room *Room) {
	t.Helper()
	if code := admin.Call(t, http.MethodPost, "/api/admin/rooms/"+room.ID.String()+"/reflect", nil, nil); code != http.StatusOK {
		t.Fatalf("asking for a pass gave %d", code)
	}
}

// reflection answers a pass over a room with script, and turns in it with the default answer
func reflection(t *testing.T, e *environment, room *Room, script fakenet.Script) {
	e.Model.Script(t, room.ID.String(), func(req fakenet.ModelRequest) fakenet.Reply {
		if req.Metadata["kind"] == "reflection" {
			return script(req)
		}
		return fakenet.Answer(fakenet.DefaultAnswer)
	})
}

// A pass over a group room forms memories of it, which reach its turns and no other room's. The passes take one
// room at a time, so these don't run in parallel.
func TestRoomMemories(t *testing.T) {
	e := setUp(t)
	alice, bob := e.User(t, "alice"), e.User(t, "bob")
	room := e.Room(t, alice, []*User{bob})
	other := e.Room(t, alice, []*User{bob})
	admin := e.Login(t, e.admin, e.Room(t, e.admin, nil))
	memory := marker(t, "memory")

	reflection(t, e, room, func(fakenet.ModelRequest) fakenet.Reply {
		return fakenet.CallTool("pass-1", "update_room_memories",
			fmt.Sprintf(`{"changes":[{"action":"add","memory":"The room talks about %s"}]}`, memory))
	})
	room.WaitForBot(t, alice.Ask(t, room, "let's talk about fish"))
	reflect(t, admin, room)
	if stored := e.Query(t, "SELECT count(*) FROM room_memory WHERE room_id = $1", room.ID.String()); stored[0] != "1" {
		t.Fatalf("%s memories of the room are stored, want the one the pass added", stored[0])
	}

	room.WaitForBot(t, alice.Ask(t, room, "what do we talk about?"))
	assertSaw(t, e, room, memory, true)
	other.WaitForBot(t, alice.Ask(t, other, "what do we talk about?"))
	assertSaw(t, e, other, memory, false)
}

// A direct chat gets no memories, and no pass asks the model about it
func TestNoRoomMemoriesOfDirectChats(t *testing.T) {
	e := setUp(t)
	alice := e.User(t, "alice")
	dm := e.Room(t, alice, nil)
	admin := e.Login(t, e.admin, e.Room(t, e.admin, nil))
	reflection(t, e, dm, func(fakenet.ModelRequest) fakenet.Reply {
		return fakenet.CallTool("pass-1", "update_room_memories", `{"changes":[{"action":"add","memory":"a note on Alice"}]}`)
	})

	dm.WaitForBot(t, alice.Ask(t, dm, "remember this"))
	reflect(t, admin, dm)
	for _, req := range e.Model.Requests(dm.ID.String()) {
		if req.Metadata["kind"] == "reflection" {
			t.Error("a pass over a direct chat asked the model")
		}
	}
	if stored := e.Query(t, "SELECT count(*) FROM room_memory WHERE room_id = $1", dm.ID.String()); stored[0] != "0" {
		t.Errorf("%s memories of a direct chat are stored", stored[0])
	}
}

// Once a group room has been quiet for room_memory_delay, the bot thinks back over it without anyone asking. A
// direct chat that has been as quiet gets no pass. This changes a setting and restarts the bot, so it doesn't run
// in parallel.
func TestQuietRoomsAreThoughtBackOver(t *testing.T) {
	e := setUp(t)
	alice, bob := e.User(t, "alice"), e.User(t, "bob")
	room := e.Room(t, alice, []*User{bob})
	dm := e.Room(t, alice, nil)
	memory := marker(t, "memory")
	reflection(t, e, room, func(fakenet.ModelRequest) fakenet.Reply {
		return fakenet.CallTool("pass-1", "update_room_memories",
			fmt.Sprintf(`{"changes":[{"action":"add","memory":"The room talks about %s"}]}`, memory))
	})
	reflection(t, e, dm, func(fakenet.ModelRequest) fakenet.Reply {
		return fakenet.CallTool("pass-1", "update_room_memories", `{"changes":[{"action":"add","memory":"a note on Alice"}]}`)
	})
	room.WaitForBot(t, alice.Ask(t, room, "let's talk about fish"))
	dm.WaitForBot(t, alice.Ask(t, dm, "remember this"))

	// room_memory_delay can't be set below five minutes, so the test adds a configuration with a shorter one, as the
	// admin's command would, and puts the configuration back after. Its cleanup runs after the bot's restart's.
	original := e.Query(t, "SELECT max(id) FROM chat_config")[0]
	e.Query(t, `INSERT INTO chat_config (model, context_high_tokens, context_low_tokens, max_tool_iterations,
			max_response_tokens, image_detail, web_part_bytes, turn_timeout, model_timeout, model_attempts, room_memory_delay)
		SELECT model, context_high_tokens, context_low_tokens, max_tool_iterations, max_response_tokens, image_detail,
			web_part_bytes, turn_timeout, model_timeout, model_attempts, INTERVAL '1 second'
		FROM chat_config WHERE id = $1::bigint RETURNING id`, original)
	adminRoom := e.Room(t, e.admin, nil)
	t.Cleanup(func() { adminRoom.WaitForBot(t, e.admin.Say(t, adminRoom, "!chat config revert "+original)) })
	e.RestartBotWith(t, "SIIKABOT_E2E_REFLECTION_INTERVAL=1s")

	eventually(t, "a pass over both rooms", func() bool {
		return e.Query(t, "SELECT count(*) FROM room_memory_progress WHERE room_id IN ($1, $2)", room.ID.String(), dm.ID.String())[0] == "2"
	})
	if stored := e.Query(t, "SELECT count(*) FROM room_memory WHERE room_id = $1", room.ID.String())[0]; stored != "1" {
		t.Errorf("%s memories of the quiet room are stored, want the one its pass added", stored)
	}
	if stored := e.Query(t, "SELECT count(*) FROM room_memory WHERE room_id = $1", dm.ID.String())[0]; stored != "0" {
		t.Errorf("%s memories of a direct chat are stored", stored)
	}
	for _, req := range e.Model.Requests(dm.ID.String()) {
		if req.Metadata["kind"] == "reflection" {
			t.Error("a pass over a direct chat asked the model")
		}
	}
}

// A pass whose answer was cut off doesn't count as having thought back over the conversations (B1)
func TestCutOffPassDoesntAdvance(t *testing.T) {
	knownBug(t, "B1", "a pass whose answer was cut off counts as a clean unchanged pass")
	e := setUp(t)
	alice, bob := e.User(t, "alice"), e.User(t, "bob")
	room := e.Room(t, alice, []*User{bob})
	admin := e.Login(t, e.admin, e.Room(t, e.admin, nil))
	reflection(t, e, room, func(fakenet.ModelRequest) fakenet.Reply { return fakenet.Reply{FinishReason: "length"} })

	room.WaitForBot(t, alice.Ask(t, room, "let's talk about fish"))
	reflect(t, admin, room)
	if progress := e.Query(t, "SELECT reflected_until FROM room_memory_progress WHERE room_id = $1", room.ID.String()); len(progress) == 1 && progress[0] != "0" {
		t.Errorf("a cut-off pass moved the progress to %s", progress[0])
	}
}

// Once the history is over context_high_tokens, the window moves on to a turn boundary within context_low_tokens,
// and the turns after it start from there
func TestContextWindowMovesOn(t *testing.T) {
	e := setUp(t)
	e.changeSettings(t, "context_low_tokens 150", "context_high_tokens 300")
	alice := e.User(t, "alice")
	room := e.Room(t, alice, nil)
	// About 90 tokens a question, so the window goes over the high mark in a few turns
	padding := strings.Repeat("words to fill the window ", 14)
	var questions []string
	for i := range 5 {
		question := marker(t, fmt.Sprintf("q%d", i))
		questions = append(questions, question)
		room.WaitForBot(t, alice.Ask(t, room, question+" "+padding))
	}

	requests := e.Model.Requests(room.ID.String())
	if len(requests) != len(questions) {
		t.Fatalf("the model got %d requests, want one a turn", len(requests))
	}
	moved := -1
	for i, req := range requests {
		if !strings.Contains(req.Text(), questions[i]) {
			t.Errorf("turn %d didn't show the model its own question", i)
		}
		if moved < 0 && !strings.Contains(req.Text(), questions[0]) {
			moved = i
		}
	}
	if moved < 0 {
		t.Fatal("the window never moved on")
	}
	if moved == len(requests)-1 {
		t.Fatal("the window moved on only in the last turn, so nothing shows it stays moved")
	}
	// The turn after the move starts from where the window moved to, rather than working it out again
	if next := requests[moved+1].Text(); !strings.Contains(next, questions[moved]) || strings.Contains(next, questions[0]) {
		t.Errorf("the turn after the window moved on, at turn %d, doesn't start from there", moved)
	}
	if anchors := e.Query(t, "SELECT count(*) FROM chat_context_anchors WHERE room_id = $1", room.ID.String())[0]; anchors != "1" {
		t.Errorf("the room has %s context anchors, want the one the window moved to", anchors)
	}
}

// A tool that fails gives the model the error to work with, and the turn still ends with an answer
func TestToolFailureReachesTheModel(t *testing.T) {
	e := setUp(t)
	t.Parallel()
	alice := e.User(t, "alice")
	room := e.Room(t, alice, nil)

	e.Model.Script(t, room.ID.String(), toolThenAnswer("dns_lookup", `{"domain":"example.com","record_type":"SRV"}`,
		"the lookup failed"))
	room.WaitForBotMessage(t, alice.Ask(t, room, "look up example.com"), bodyContains("the lookup failed"))
	var result string
	for _, req := range e.Model.Requests(room.ID.String()) {
		if r, ok := req.TurnToolResults()["call-1"]; ok {
			result = r
		}
	}
	if !strings.Contains(result, "Error executing dns_lookup") {
		t.Errorf("the model was given %q for the failed call", result)
	}
}

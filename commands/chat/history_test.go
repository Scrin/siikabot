package chat

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Scrin/siikabot/aigateway"
	"github.com/Scrin/siikabot/db"
)

func ptr(s string) *string { return &s }

// textMsg builds a plain conversation row. A user row gets the attribution that every stored user
// row carries.
func textMsg(role, message string, at time.Time) db.ChatMessage {
	msg := db.ChatMessage{Role: role, Message: message, MessageType: "text", Timestamp: at}
	if role == "user" {
		name, unseen := "Alice", 0
		msg.UserID = "@alice:example.com"
		msg.SenderName = &name
		msg.SentAt = &at
		msg.Mentions = []db.Mention{}
		msg.UnseenBefore = &unseen
	}
	return msg
}

// callMsg builds a tool_call row
func callMsg(id, name, args string, at time.Time) db.ChatMessage {
	return db.ChatMessage{
		Role: "assistant", Message: args, MessageType: "tool_call",
		ToolCallID: ptr(id), ToolName: ptr(name), Timestamp: at,
	}
}

// responseMsg builds a tool_response row
func responseMsg(id, name, response string, at time.Time) db.ChatMessage {
	return db.ChatMessage{
		Role: "tool", Message: response, MessageType: "tool_response",
		ToolCallID: ptr(id), ToolName: ptr(name), Timestamp: at,
	}
}

// summarise renders the rebuilt messages compactly so ordering assertions stay readable
func summarise(messages []aigateway.Message) []string {
	out := make([]string, 0, len(messages))
	for _, msg := range messages {
		switch {
		case len(msg.ToolCalls) > 0:
			names := ""
			for i, call := range msg.ToolCalls {
				if i > 0 {
					names += "+"
				}
				names += call.Function.Name
			}
			out = append(out, "assistant[tool_calls:"+names+"]")
		case msg.Role == "tool":
			out = append(out, "tool["+msg.ToolCallID+"]")
		default:
			content, _ := msg.Content.(string)
			// A user message is replayed with its header; the message itself is on the last line
			if msg.Role == "user" {
				content = content[strings.LastIndex(content, "\n")+1:]
			}
			out = append(out, msg.Role+"["+content+"]")
		}
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestHistoryPreservesChronologicalOrder is the A1 regression test. The previous implementation
// emitted every text message first and every tool-call group afterwards, so tool calls always
// landed at the end regardless of when they happened.
func TestHistoryPreservesChronologicalOrder(t *testing.T) {
	base := time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)
	at := func(offset int) time.Time { return base.Add(time.Duration(offset) * time.Second) }

	history := []db.ChatMessage{
		textMsg("user", "what is the weather?", at(0)),
		callMsg("call_1", "get_weather", `{"location":"Oulu"}`, at(1)),
		responseMsg("call_1", "get_weather", "-7 C", at(2)),
		textMsg("assistant", "it is -7 C", at(3)),
		textMsg("user", "and the news?", at(4)),
		callMsg("call_2", "get_news", `{}`, at(5)),
		responseMsg("call_2", "get_news", "some headlines", at(6)),
		textMsg("assistant", "here are the headlines", at(7)),
	}

	var messages []aigateway.Message
	processHistoryMessages(context.Background(), history, &messages)

	want := []string{
		"user[what is the weather?]",
		"assistant[tool_calls:get_weather]",
		"tool[call_1]",
		"assistant[it is -7 C]",
		"user[and the news?]",
		"assistant[tool_calls:get_news]",
		"tool[call_2]",
		"assistant[here are the headlines]",
	}

	if got := summarise(messages); !equal(got, want) {
		t.Errorf("conversation order not preserved\n got: %v\nwant: %v", got, want)
	}
}

// TestHistoryGroupsParallelToolCalls verifies a batch of calls issued in one turn is replayed as a
// single assistant message followed by one tool reply each.
func TestHistoryGroupsParallelToolCalls(t *testing.T) {
	base := time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)
	at := func(offset int) time.Time { return base.Add(time.Duration(offset) * time.Second) }

	history := []db.ChatMessage{
		textMsg("user", "weather and time?", at(0)),
		callMsg("call_1", "get_weather", `{}`, at(1)),
		callMsg("call_2", "get_time", `{}`, at(1)),
		responseMsg("call_1", "get_weather", "-7 C", at(2)),
		responseMsg("call_2", "get_time", "14:05", at(2)),
		textMsg("assistant", "-7 C at 14:05", at(3)),
	}

	var messages []aigateway.Message
	processHistoryMessages(context.Background(), history, &messages)

	want := []string{
		"user[weather and time?]",
		"assistant[tool_calls:get_weather+get_time]",
		"tool[call_1]",
		"tool[call_2]",
		"assistant[-7 C at 14:05]",
	}

	if got := summarise(messages); !equal(got, want) {
		t.Errorf("parallel tool calls not grouped correctly\n got: %v\nwant: %v", got, want)
	}
}

// TestHistoryHandlesMultipleIterations verifies consecutive tool iterations stay separate rather
// than collapsing into one assistant message.
func TestHistoryHandlesMultipleIterations(t *testing.T) {
	base := time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)
	at := func(offset int) time.Time { return base.Add(time.Duration(offset) * time.Second) }

	history := []db.ChatMessage{
		textMsg("user", "research this", at(0)),
		callMsg("call_1", "web_search", `{}`, at(1)),
		responseMsg("call_1", "web_search", "results", at(2)),
		callMsg("call_2", "web", `{}`, at(3)),
		responseMsg("call_2", "web", "page content", at(4)),
		textMsg("assistant", "here is what I found", at(5)),
	}

	var messages []aigateway.Message
	processHistoryMessages(context.Background(), history, &messages)

	want := []string{
		"user[research this]",
		"assistant[tool_calls:web_search]",
		"tool[call_1]",
		"assistant[tool_calls:web]",
		"tool[call_2]",
		"assistant[here is what I found]",
	}

	if got := summarise(messages); !equal(got, want) {
		t.Errorf("iterations not kept separate\n got: %v\nwant: %v", got, want)
	}
}

// TestHistoryDropsOrphanedToolResponse verifies a response whose call fell outside the window is
// skipped rather than emitted with no preceding tool_calls message.
func TestHistoryDropsOrphanedToolResponse(t *testing.T) {
	base := time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)
	at := func(offset int) time.Time { return base.Add(time.Duration(offset) * time.Second) }

	history := []db.ChatMessage{
		responseMsg("call_cut", "get_weather", "-7 C", at(0)),
		textMsg("assistant", "it is -7 C", at(1)),
		textMsg("user", "thanks", at(2)),
	}

	var messages []aigateway.Message
	processHistoryMessages(context.Background(), history, &messages)

	want := []string{"assistant[it is -7 C]", "user[thanks]"}

	if got := summarise(messages); !equal(got, want) {
		t.Errorf("orphaned tool response not dropped\n got: %v\nwant: %v", got, want)
	}
}

// TestHistoryReplaysStoredBatchesAsOneStep is the F12 regression test. The batch is built in the
// order the writer stores it, since a test built in the order the replay expects is what let the
// writer's old order go unnoticed: each call followed by its response came back as calls made one
// at a time.
func TestHistoryReplaysStoredBatchesAsOneStep(t *testing.T) {
	base := time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)
	turn := db.Turn{RoomID: "!room:example.com", EventID: "$trigger"}

	history := []db.ChatMessage{textMsg("user", "weather and time?", base)}
	history = append(history, db.ToolCallRows(turn, testBotUserID, []db.ToolCallRecord{
		{ToolCallID: "call_1", ToolName: "get_weather", Arguments: `{}`, Response: "-7 C"},
		{ToolCallID: "call_2", ToolName: "get_time", Arguments: `{}`, Response: "14:05"},
	})...)
	history = append(history, textMsg("assistant", "-7 C at 14:05", base))

	var messages []aigateway.Message
	processHistoryMessages(context.Background(), history, &messages)

	want := []string{
		"user[weather and time?]",
		"assistant[tool_calls:get_weather+get_time]",
		"tool[call_1]",
		"tool[call_2]",
		"assistant[-7 C at 14:05]",
	}

	if got := summarise(messages); !equal(got, want) {
		t.Errorf("stored batch not replayed as one step\n got: %v\nwant: %v", got, want)
	}
}

// TestHistoryReplaysUserTurnsWithTheirHeader verifies a user message comes back exactly as the
// current turn showed it
func TestHistoryReplaysUserTurnsWithTheirHeader(t *testing.T) {
	at := time.Date(2026, 9, 25, 14, 2, 0, 0, time.UTC)
	row := textMsg("user", "what's the weather?", at)

	var messages []aigateway.Message
	processHistoryMessages(context.Background(), []db.ChatMessage{row}, &messages)

	want := renderUserTurn(row.UserTurn(), false)
	if got, _ := messages[0].Content.(string); got != want {
		t.Errorf("replayed user message = %q, want %q", got, want)
	}
	if !strings.HasPrefix(want, "[Alice (@alice:example.com) · 2026-09-25 14:02]\n") {
		t.Errorf("replayed user message lacks its header: %q", want)
	}
}

func TestHistoryEmpty(t *testing.T) {
	var messages []aigateway.Message
	processHistoryMessages(context.Background(), nil, &messages)

	if len(messages) != 0 {
		t.Errorf("expected no messages, got %d", len(messages))
	}
}

// TestHistoryAppendsToExistingMessages verifies the system prompt already in the slice is preserved.
func TestHistoryAppendsToExistingMessages(t *testing.T) {
	base := time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)

	messages := []aigateway.Message{{Role: "system", Content: "you are a bot"}}
	processHistoryMessages(context.Background(), []db.ChatMessage{textMsg("user", "hello", base)}, &messages)

	want := []string{"system[you are a bot]", "user[hello]"}

	if got := summarise(messages); !equal(got, want) {
		t.Errorf("existing messages not preserved\n got: %v\nwant: %v", got, want)
	}
}

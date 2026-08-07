package chat

import (
	"context"
	"testing"
	"time"

	"github.com/Scrin/siikabot/aigateway"
	"github.com/Scrin/siikabot/db"
)

func ptr(s string) *string { return &s }

// textMsg builds a plain conversation row
func textMsg(role, message string, at time.Time) db.ChatMessage {
	return db.ChatMessage{Role: role, Message: message, MessageType: "text", Timestamp: at}
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

// TestHistoryDropsOrphanedToolCall is the A2 repair test. A call with no response must not be
// replayed: the chat API rejects an assistant message whose tool_calls are not all answered, which
// would break every later request in the room.
func TestHistoryDropsOrphanedToolCall(t *testing.T) {
	base := time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)
	at := func(offset int) time.Time { return base.Add(time.Duration(offset) * time.Second) }

	history := []db.ChatMessage{
		textMsg("user", "weather?", at(0)),
		callMsg("call_orphan", "get_weather", `{}`, at(1)),
		textMsg("user", "still there?", at(2)),
	}

	var messages []aigateway.Message
	processHistoryMessages(context.Background(), history, &messages)

	want := []string{"user[weather?]", "user[still there?]"}

	if got := summarise(messages); !equal(got, want) {
		t.Errorf("orphaned tool call not dropped\n got: %v\nwant: %v", got, want)
	}
}

// TestHistoryDropsOnlyTheUnansweredCall verifies a partially answered batch keeps the answered
// calls rather than discarding the whole turn.
func TestHistoryDropsOnlyTheUnansweredCall(t *testing.T) {
	base := time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)
	at := func(offset int) time.Time { return base.Add(time.Duration(offset) * time.Second) }

	history := []db.ChatMessage{
		textMsg("user", "weather and time?", at(0)),
		callMsg("call_1", "get_weather", `{}`, at(1)),
		callMsg("call_2", "get_time", `{}`, at(1)),
		responseMsg("call_1", "get_weather", "-7 C", at(2)),
		textMsg("assistant", "it is -7 C", at(3)),
	}

	var messages []aigateway.Message
	processHistoryMessages(context.Background(), history, &messages)

	want := []string{
		"user[weather and time?]",
		"assistant[tool_calls:get_weather]",
		"tool[call_1]",
		"assistant[it is -7 C]",
	}

	if got := summarise(messages); !equal(got, want) {
		t.Errorf("unanswered call not dropped in isolation\n got: %v\nwant: %v", got, want)
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

// TestHistoryHandlesLegacyRowsWithoutMessageType covers rows predating the message_type column,
// which default to an empty string rather than "text".
func TestHistoryHandlesLegacyRowsWithoutMessageType(t *testing.T) {
	base := time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)

	history := []db.ChatMessage{
		{Role: "user", Message: "hello", MessageType: "", Timestamp: base},
		{Role: "assistant", Message: "hi", MessageType: "", Timestamp: base.Add(time.Second)},
	}

	var messages []aigateway.Message
	processHistoryMessages(context.Background(), history, &messages)

	want := []string{"user[hello]", "assistant[hi]"}

	if got := summarise(messages); !equal(got, want) {
		t.Errorf("legacy rows mishandled\n got: %v\nwant: %v", got, want)
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

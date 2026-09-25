package db

import (
	"testing"
	"time"
)

// ToolCallRows stores a batch calls first, then responses, which is the order the replay reads a
// batch back in. Each call followed by its response came back as calls made one at a time.
func TestToolCallRowsStoreCallsBeforeResponses(t *testing.T) {
	turn := Turn{RoomID: "!room:x", EventID: "$trigger", ThreadRootID: "$root"}
	rows := ToolCallRows(turn, "@bot:x", []ToolCallRecord{
		{ToolCallID: "call_1", ToolName: "get_weather", Arguments: "{}", Response: "-7 C", ValidityDuration: time.Hour},
		{ToolCallID: "call_2", ToolName: "get_time", Arguments: "{}", Response: "14:05"},
	})

	want := []string{"tool_call:call_1", "tool_call:call_2", "tool_response:call_1", "tool_response:call_2"}
	if len(rows) != len(want) {
		t.Fatalf("got %d rows, want %d", len(rows), len(want))
	}
	for i, row := range rows {
		if got := row.MessageType + ":" + *row.ToolCallID; got != want[i] {
			t.Errorf("row %d = %s, want %s", i, got, want[i])
		}
		if row.TurnEventID != "$trigger" || stringOrEmpty(row.ThreadRootID) != "$root" {
			t.Errorf("row %d doesn't belong to the turn: %q %v", i, row.TurnEventID, row.ThreadRootID)
		}
	}

	// A call and its response share an expiry, so they always leave the history together
	if rows[0].Expiry == nil || rows[2].Expiry == nil || !rows[0].Expiry.Equal(*rows[2].Expiry) {
		t.Error("a call and its response don't share an expiry")
	}
	if rows[1].Expiry != nil {
		t.Error("a call without a validity got an expiry")
	}
}

func TestUserTurnReadsBackAUserRow(t *testing.T) {
	sentAt := time.Date(2026, 9, 25, 14, 2, 0, 0, time.UTC)
	name, unseen := "Bob", 3
	quoteID, quoteSender, quoteName, quoteKind := "$carol", "@carol:x", "Carol", QuoteImage

	row := ChatMessage{
		RoomID: "!room:x", UserID: "@bob:x", Message: "what is this?", Role: "user", MessageType: "text",
		TurnEventID: "$bob", SenderName: &name, SentAt: &sentAt, Mentions: []Mention{}, UnseenBefore: &unseen,
		ReplyToEventID: &quoteID, ReplyToSender: &quoteSender, ReplyToSenderName: &quoteName,
		ReplyToSentAt: &sentAt, ReplyToKind: &quoteKind,
	}

	turn := row.UserTurn()
	if turn.EventID != "$bob" || turn.ThreadRootID != "" || turn.SenderName != "Bob" || turn.UnseenBefore != 3 {
		t.Errorf("UserTurn() = %#v", turn)
	}
	if turn.ReplyTo == nil || turn.ReplyTo.Kind != QuoteImage || turn.ReplyTo.Body != "" || turn.ReplyTo.SenderName != "Carol" {
		t.Errorf("UserTurn().ReplyTo = %#v", turn.ReplyTo)
	}
}

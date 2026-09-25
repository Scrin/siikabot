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
	name, unseen, hasImage := "Bob", 3, true
	carol := QuotedMessage{EventID: "$carol", Sender: "@carol:x", SenderName: "Carol", SentAt: sentAt, Kind: QuoteImage}
	dave := QuotedMessage{EventID: "$dave", Sender: "@dave:x", SenderName: "Dave", SentAt: sentAt, Kind: QuoteText, Body: "see this"}
	erin := QuotedMessage{EventID: "$erin", Sender: "@erin:x", SenderName: "Erin", SentAt: sentAt, Kind: QuoteDeleted}

	row := ChatMessage{
		RoomID: "!room:x", UserID: "@bob:x", Message: "what is this?", Role: "user", MessageType: "text",
		TurnEventID: "$bob", SenderName: &name, SentAt: &sentAt, Mentions: []Mention{}, UnseenBefore: &unseen,
		HasImage: &hasImage,
		References: []Reference{
			{Relation: RelationReply, QuotedMessage: carol},
			{Relation: RelationLink, QuotedMessage: dave},
			{Relation: RelationLink, QuotedMessage: erin},
		},
	}

	turn := row.UserTurn()
	if turn.EventID != "$bob" || turn.ThreadRootID != "" || turn.SenderName != "Bob" || turn.UnseenBefore != 3 || !turn.HasImage {
		t.Errorf("UserTurn() = %#v", turn)
	}
	if turn.ReplyTo == nil || *turn.ReplyTo != carol {
		t.Errorf("UserTurn().ReplyTo = %#v, want Carol's image", turn.ReplyTo)
	}
	if len(turn.Links) != 2 || turn.Links[0] != dave || turn.Links[1] != erin {
		t.Errorf("UserTurn().Links = %#v, want Dave's and Erin's messages, in order", turn.Links)
	}

	// Stored the way it reads back: the reply first, then the links in order
	references := turn.references()
	if len(references) != 3 || references[0].Relation != RelationReply || references[1].EventID != "$dave" || references[2].EventID != "$erin" {
		t.Errorf("references() = %#v", references)
	}
}

func TestUserTurnWithoutReferences(t *testing.T) {
	turn := UserTurn{Turn: Turn{RoomID: "!room:x", EventID: "$bob"}, Message: "hi"}
	if references := turn.references(); len(references) != 0 {
		t.Errorf("references() = %#v, want none", references)
	}
}

func TestTurnTimeline(t *testing.T) {
	turn := Turn{RoomID: "!room:x", EventID: "$bob", ThreadRootID: "$root"}
	if got := turn.Timeline(); got != (Timeline{RoomID: "!room:x", ThreadRootID: "$root"}) {
		t.Errorf("Timeline() = %#v", got)
	}
}

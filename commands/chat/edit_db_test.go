package chat

import (
	"context"
	"testing"
	"time"

	"github.com/Scrin/siikabot/db"
	"github.com/Scrin/siikabot/testutil/dbtest"
)

// Edits followed while a turn ran, before it stored its question and what it quotes, are followed again once
// they're stored. Only the sender's own edits count (P2).
func TestPersistFollowsEditsMadeMeanwhile(t *testing.T) {
	connString := dbtest.Use(t)
	ctx := context.Background()
	alice, bob, mallory := "@alice:example.com", "@bob:example.com", "@mallory:example.com"
	turn := db.Turn{RoomID: "!edits:example.com", EventID: "$edits-question"}
	quoted := "$edits-quoted"
	now := time.Now()
	question := db.UserTurn{
		Turn: turn, UserID: alice, SenderName: "Alice", SentAt: now, Message: "the question as sent",
		ReplyTo: &db.QuotedMessage{EventID: quoted, Sender: bob, SenderName: "Bob", SentAt: now, Kind: db.QuoteText,
			Body: "the quote as sent"},
	}

	FollowEdit(ctx, turn.RoomID, turn.EventID, "$edits-1", alice, "the question as edited", "siikabot: the question as edited")
	FollowEdit(ctx, turn.RoomID, turn.EventID, "$edits-2", mallory, "Mallory's question", "Mallory's question")
	FollowEdit(ctx, turn.RoomID, quoted, "$edits-3", bob, "the quote as edited", "the quote as edited")
	FollowEdit(ctx, turn.RoomID, quoted, "$edits-4", mallory, "Mallory's quote", "Mallory's quote")
	if edit, ok := latestEdit(quoted, bob); !ok || edit.text != "the quote as edited" {
		t.Errorf("Bob's latest edit is %+v, %v", edit, ok)
	}

	persist(ctx, turn, "user_turn", func(ctx context.Context) error { return db.SaveUserTurn(ctx, question) },
		storedEvents(question)...)
	if got := queryOne[string](t, connString, "SELECT message FROM chat_history WHERE turn_event_id = $1", turn.EventID); got != "the question as edited" {
		t.Errorf("the question is stored as %q", got)
	}
	if got := queryOne[string](t, connString, "SELECT body FROM chat_references WHERE event_id = $1", quoted); got != "the quote as edited" {
		t.Errorf("the quote is stored as %q", got)
	}
}

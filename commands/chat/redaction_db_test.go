package chat

import (
	"context"
	"testing"
	"time"

	"github.com/Scrin/siikabot/db"
	"github.com/Scrin/siikabot/testutil/dbtest"
	"github.com/jackc/pgx/v5"
)

// queryOne reads one value from the test's database
func queryOne[T any](t *testing.T, connString, sql string, args ...any) T {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, connString)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	var value T
	if err := conn.QueryRow(ctx, sql, args...).Scan(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

// A turn's writes store what other messages hold: the ones its question quotes and shows the images of, and its
// answer. One of them redacted while the turn ran was handled before the write, when there was nothing to remove,
// so the write removes its part again (P1).
func TestPersistForgetsWhatWasRedactedMeanwhile(t *testing.T) {
	connString := dbtest.Use(t)
	ctx := context.Background()
	turn := db.Turn{RoomID: "!persist:example.com", EventID: "$persist-question"}
	quoted, linked, answer := "$persist-quoted", "$persist-linked", "$persist-answer"
	now := time.Now()
	question := db.UserTurn{
		Turn: turn, UserID: "@alice:example.com", SenderName: "Alice", SentAt: now, Message: "what are these?",
		ReplyTo: &db.QuotedMessage{EventID: quoted, Sender: "@bob:example.com", SenderName: "Bob", SentAt: now,
			Kind: db.QuoteText, Body: "a secret"},
		Links: []db.QuotedMessage{{EventID: linked, Sender: "@bob:example.com", SenderName: "Bob", SentAt: now,
			Kind: db.QuoteImage, Body: "a secret caption"}},
		Images: []db.ChatImage{{EventID: linked, MIMEType: "image/png", Width: 1, Height: 1, Data: []byte("png")}},
	}

	markRedacted(quoted)
	markRedacted(linked)
	persist(ctx, turn, "user_turn", func(ctx context.Context) error { return db.SaveUserTurn(ctx, question) },
		storedEvents(question)...)
	if n := queryOne[int](t, connString, "SELECT count(*) FROM chat_history WHERE turn_event_id = $1", turn.EventID); n != 1 {
		t.Errorf("the question has %d rows, want it stored", n)
	}
	if n := queryOne[int](t, connString, "SELECT count(*) FROM chat_references WHERE kind <> 'deleted' OR body IS NOT NULL"); n != 0 {
		t.Errorf("%d references still hold what was redacted", n)
	}
	if n := queryOne[int](t, connString, "SELECT count(*) FROM chat_images"); n != 0 {
		t.Errorf("%d images of redacted messages are stored", n)
	}

	// The answer's redaction takes its turn with it, as it does when it comes after the answer is stored
	markRedacted(answer)
	persist(ctx, turn, "answer", func(ctx context.Context) error {
		return db.SaveAnswer(ctx, turn, testBotUserID, "an answer", answer)
	}, answer)
	if n := queryOne[int](t, connString, "SELECT count(*) FROM chat_history WHERE room_id = $1", turn.RoomID); n != 0 {
		t.Errorf("%d rows of a turn whose answer was redacted are stored", n)
	}
}

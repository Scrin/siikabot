package db

import (
	"context"
	"errors"
	"io/fs"
	"strings"
	"sync"
	"testing"
	"time"
)

// migrationFiles lists the migrations
func migrationFiles(t *testing.T) []string {
	t.Helper()
	entries, err := fs.ReadDir(migrations, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".sql") {
			names = append(names, entry.Name())
		}
	}
	return names
}

// Every migration runs on an empty database, and running them again changes nothing
func TestMigrationsRunOnAnEmptyDatabase(t *testing.T) {
	p := useEmptyTestDatabase(t)
	ctx := context.Background()
	if err := runMigrations(ctx, p, allMigrations); err != nil {
		t.Fatalf("running the migrations: %v", err)
	}

	var recorded int
	var lastRun time.Time
	if err := p.QueryRow(ctx, "SELECT count(*), max(executed_at) FROM migrations").Scan(&recorded, &lastRun); err != nil {
		t.Fatal(err)
	}
	if want := len(migrationFiles(t)); recorded != want {
		t.Errorf("%d migrations are recorded, want all %d", recorded, want)
	}

	if err := runMigrations(ctx, p, allMigrations); err != nil {
		t.Fatalf("running the migrations again: %v", err)
	}
	var again time.Time
	_ = p.QueryRow(ctx, "SELECT max(executed_at) FROM migrations").Scan(&again)
	if !again.Equal(lastRun) {
		t.Error("running the migrations again ran some of them")
	}
}

// A migration that has run can't be changed: the bot refuses to start rather than run a database whose schema
// differs from what the files say
func TestChangedMigrationIsRefused(t *testing.T) {
	p := useEmptyTestDatabase(t)
	ctx := context.Background()
	if err := runMigrations(ctx, p, allMigrations); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, "UPDATE migrations SET hash = 'changed' WHERE name = $1", migrationFiles(t)[0]); err != nil {
		t.Fatal(err)
	}
	if err := runMigrations(ctx, p, allMigrations); err == nil {
		t.Error("a migration changed after it ran wasn't noticed")
	}
}

// room_memory_progress records how far the bot has thought back over a room as a chat_history id, so the ids
// mustn't start over when the history is cleared (CLAUDE.md, "Clearing the chat history")
func TestMigrationsNeverRestartIdentity(t *testing.T) {
	for _, name := range migrationFiles(t) {
		content, err := fs.ReadFile(migrations, "migrations/"+name)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(content), "\n") {
			code, _, _ := strings.Cut(line, "--") // a comment may say not to
			if strings.Contains(strings.ToUpper(code), "RESTART IDENTITY") {
				t.Errorf("%s:%d restarts an identity", name, i+1)
			}
		}
	}
}

// Clearing the chat history empties the chat context and keeps everything people saved, and the bot's own
// memories of rooms with their progress. The latest migration that clears it is run on a database with one of
// everything.
func TestClearingTheChatHistoryKeepsWhatPeopleSaved(t *testing.T) {
	const clearing = "0034__label-untrusted-tool-results.sql"
	p := useEmptyTestDatabase(t)
	ctx := context.Background()
	if err := runMigrations(ctx, p, migrationsBefore(clearing)); err != nil {
		t.Fatal(err)
	}

	turn := question(roomA, "$q1", alice, "hi")
	turn.ReplyTo = &QuotedMessage{EventID: "$x", Sender: bob, SenderName: "bob", SentAt: chatT0, Kind: QuoteText, Body: "quoted"}
	turn.Images = []ChatImage{{EventID: "$q1", MIMEType: "image/png", Width: 1, Height: 1, Data: []byte("x")}}
	storeTurn(t, turn, "get_time", "hello", "$a1")
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(SetChatContextAnchor(ctx, Timeline{RoomID: roomA}, 1))
	addReminder(t, alice, roomA, "dentist", time.Hour)
	must(SaveMemory(ctx, alice, "likes cats", MemoryView{RoomID: roomA}))
	_, err := ApplyRoomMemoryChanges(ctx, roomA, []RoomMemoryChange{{Action: RoomMemoryAdd, Memory: "talks about fish"}},
		RoomMemoryProgress{RoomID: roomA, ReflectedUntil: 4, LastAttemptAt: chatT0}, nil)
	must(err)
	must(SaveChatUsage(ctx, ChatUsage{Kind: UsageTurn, RoomID: roomA, UserID: alice, Model: "m", Outcome: "ok"}))
	must(SetWebSessionToken(ctx, alice, "token"))

	if err := migrate(); err != nil {
		t.Fatal(err)
	}

	for table, want := range map[string]int{
		"chat_history": 0, "chat_references": 0, "chat_images": 0, "chat_context_anchors": 0,
		"reminders": 1, "user_memory": 1, "room_memory": 1, "room_memory_progress": 1, "chat_usage": 1,
		"user_authorizations": 1, "chat_config": 1,
	} {
		if got := count(t, table); got != want {
			t.Errorf("%s has %d rows after the history was cleared, want %d", table, got, want)
		}
	}

	// The ids carry on from where they were
	storeTurn(t, question(roomA, "$q2", alice, "again"), "", "hello again", "$a2")
	if rows := history(t, Timeline{RoomID: roomA}, ""); rows[0].ID <= 4 {
		t.Errorf("the history's ids started over at %d", rows[0].ID)
	}
}

// Configuration rows are only ever added, and changes made at the same moment both take
func TestUpdateChatConfig(t *testing.T) {
	useTestDatabase(t)
	ctx := context.Background()
	seeded, err := GetChatConfig(ctx)
	if err != nil {
		t.Fatal(err)
	}

	refusal := errors.New("not allowed")
	if _, err := UpdateChatConfig(ctx, func(cfg *ChatConfig) error { return refusal }); !errors.Is(err, refusal) {
		t.Errorf("a refused change returned %v", err)
	}
	if _, err := UpdateChatConfig(ctx, func(cfg *ChatConfig) error { cfg.ContextLowTokens = -1; return nil }); err == nil {
		t.Error("a value the table's checks refuse was written")
	}
	if history, _ := GetChatConfigHistory(ctx); len(history) != 1 {
		t.Fatalf("refused changes added rows: %d", len(history))
	}

	var wg sync.WaitGroup
	for _, change := range []func(*ChatConfig){
		func(cfg *ChatConfig) { cfg.MaxToolIterations = seeded.MaxToolIterations + 1 },
		func(cfg *ChatConfig) { cfg.MaxResponseTokens = seeded.MaxResponseTokens + 1 },
	} {
		wg.Go(func() {
			if _, err := UpdateChatConfig(ctx, func(cfg *ChatConfig) error { change(cfg); return nil }); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()

	latest, _ := GetChatConfig(ctx)
	if latest.MaxToolIterations != seeded.MaxToolIterations+1 || latest.MaxResponseTokens != seeded.MaxResponseTokens+1 {
		t.Errorf("after two changes at once the configuration is %+v, want both", latest)
	}
	if history, _ := GetChatConfigHistory(ctx); len(history) != 3 {
		t.Errorf("the history has %d rows, want the seed and the two changes", len(history))
	}
	if first, ok, _ := GetChatConfigByID(ctx, seeded.ID); !ok || first.MaxToolIterations != seeded.MaxToolIterations {
		t.Errorf("the first row is now %+v", first)
	}
}

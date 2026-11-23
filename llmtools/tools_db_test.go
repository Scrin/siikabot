package llmtools

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Scrin/siikabot/db"
	"github.com/Scrin/siikabot/testutil/dbtest"
	"github.com/Scrin/siikabot/testutil/knownbug"
)

// The tools that read and write the bot's database, on a database of the test's own

const (
	toolRoom  = "!room:example.com"
	toolOther = "!other:example.com"
	toolDM    = "!dm:example.com"
	toolAlice = "@alice:example.com"
	toolBob   = "@bob:example.com"
)

// turn is the context a chat turn gives the tools: who wrote the latest message, and in which room
func turn(sender, roomID string, isDM bool) context.Context {
	ctx := context.WithValue(context.Background(), "sender", sender)
	ctx = context.WithValue(ctx, "room_id", roomID)
	return context.WithValue(ctx, "room_is_dm", isDM)
}

func callIn(t *testing.T, ctx context.Context, handler func(context.Context, string) (string, error), arguments string) string {
	t.Helper()
	result, err := handler(ctx, arguments)
	if err != nil {
		t.Fatalf("the tool failed: %v", err)
	}
	return result
}

// The memory tool saves memories scoped to where they were saved, and deletes only what the room can see
func TestMemoryTool(t *testing.T) {
	dbtest.Use(t)
	ctx := context.Background()
	inRoom, inDM := turn(toolAlice, toolRoom, false), turn(toolAlice, toolDM, true)

	callIn(t, inRoom, handleMemoryToolCall, `{"action":"save","memory":"likes cats"}`)
	callIn(t, inDM, handleMemoryToolCall, `{"action":"save","memory":"private thing"}`)
	callIn(t, turn(toolBob, toolRoom, false), handleMemoryToolCall, `{"action":"save","memory":"bob's"}`)

	memories, _ := db.GetUserMemories(ctx, toolAlice)
	var private int64
	for _, m := range memories {
		if m.Memory == "private thing" {
			private = m.ID
		}
	}
	if result, err := handleMemoryToolCall(inRoom, `{"action":"delete","memory_id":`+itoa(private)+`}`); err == nil &&
		!strings.Contains(strings.ToLower(result), "not") {
		t.Errorf("deleting a DM's memory from a group room gave %q", result)
	}
	if left, _ := db.GetUserMemories(ctx, toolAlice); len(left) != 2 {
		t.Fatalf("a DM's memory was deleted from a group room")
	}

	callIn(t, inRoom, handleMemoryToolCall, `{"action":"clear_all"}`)
	if left, _ := db.GetUserMemories(ctx, toolAlice); len(left) != 1 || left[0].Memory != "private thing" {
		t.Errorf("clearing in the group room left %+v, want the DM's memory only", left)
	}
	if bobs, _ := db.GetUserMemories(ctx, toolBob); len(bobs) != 1 {
		t.Error("clearing Alice's memories touched Bob's")
	}

	if _, err := handleMemoryToolCall(inRoom, `{"action":"save","memory":"`+strings.Repeat("x", 501)+`"}`); err == nil {
		t.Error("a memory over the limit was saved")
	}
	if _, err := handleMemoryToolCall(context.Background(), `{"action":"save","memory":"no turn"}`); err == nil {
		t.Error("a memory was saved outside a chat turn")
	}
}

// The limit is in characters, as the tool says: Finnish text isn't cut short (C7)
func TestMemoryLimitIsInCharacters(t *testing.T) {
	knownbug.Skip(t, "C7", "memory length is counted in bytes but described as characters")
	dbtest.Use(t)
	callIn(t, turn(toolAlice, toolRoom, false), handleMemoryToolCall, `{"action":"save","memory":"`+strings.Repeat("ä", 400)+`"}`)
}

// The reminder tool lists only the reminders the author set in the room the turn is in. Creating and deleting
// post to the room, so the end-to-end tests cover those.
func TestManageRemindersToolLists(t *testing.T) {
	dbtest.Use(t)
	ctx := context.Background()
	for _, rem := range []db.Reminder{
		{UserID: toolAlice, RoomID: toolRoom, Message: "dentist"},
		{UserID: toolAlice, RoomID: toolOther, Message: "sauna"},
		{UserID: toolBob, RoomID: toolRoom, Message: "bob's"},
	} {
		rem.RemindTime = time.Now().Add(time.Hour)
		if _, err := db.AddReminder(ctx, rem); err != nil {
			t.Fatal(err)
		}
	}
	inRoom := turn(toolAlice, toolRoom, false)

	listed := callIn(t, inRoom, handleManageRemindersToolCall, `{"action":"list"}`)
	assertContains(t, listed, "dentist")
	if strings.Contains(listed, "sauna") || strings.Contains(listed, "bob's") {
		t.Errorf("listing Alice's reminders in the room gave:\n%s", listed)
	}
	if listed := callIn(t, turn(toolAlice, toolDM, true), handleManageRemindersToolCall, `{"action":"list"}`); strings.Contains(listed, "dentist") {
		t.Errorf("a DM lists a group room's reminders:\n%s", listed)
	}
	if _, err := handleManageRemindersToolCall(inRoom, `{"action":"create","time":"yesterday","message":"x"}`); err == nil {
		t.Error("a time the tool can't read gave no error")
	}
}

// The message stats come from the turn's room, whatever people the model asks about
func TestMessageStatsTool(t *testing.T) {
	dbtest.Use(t)
	ctx := context.Background()
	for range 3 {
		_ = db.UpdateMessageStats(ctx, toolRoom, toolAlice, "hello there friends")
		_ = db.UpdateRoomDailyStats(ctx, toolRoom, "hello there friends")
	}
	_ = db.UpdateMessageStats(ctx, toolRoom, toolBob, "hi")
	_ = db.UpdateRoomDailyStats(ctx, toolRoom, "hi")
	for range 9 {
		_ = db.UpdateMessageStats(ctx, toolOther, toolBob, "elsewhere")
	}
	inRoom := context.WithValue(turn(toolAlice, toolRoom, false), "room_members",
		[]RoomMember{{UserID: toolAlice, Name: "Alice"}, {UserID: toolBob, Name: "Bob"}})

	users := callIn(t, inRoom, handleMessageStatsToolCall, `{"kind":"users","user_ids":["@bob:example.com"]}`)
	assertContains(t, users, "Bob", "1 message")
	if strings.Contains(users, "9 messages") || strings.Contains(users, "10 messages") {
		t.Errorf("Bob's stats in the room count his messages elsewhere:\n%s", users)
	}
	top := callIn(t, inRoom, handleMessageStatsToolCall, `{"kind":"top"}`)
	if strings.Index(top, "Alice") < 0 || strings.Index(top, "Alice") > strings.Index(top, "Bob") {
		t.Errorf("the room's top list doesn't have Alice first:\n%s", top)
	}
	assertContains(t, callIn(t, inRoom, handleMessageStatsToolCall, `{"kind":"activity","days":7}`), "4")
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

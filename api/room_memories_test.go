package api

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Scrin/siikabot/commands/chat"
	"github.com/Scrin/siikabot/db"
)

func ptr[T any](v T) *T { return &v }

// Every room the bot remembers things about or has been thinking back over is listed with its memories,
// its name, looked up once, and how it stands with the passes. Direct chats are left out, and the room
// with the most recent activity comes first.
func TestRoomMemoryEntries(t *testing.T) {
	early := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	late := time.Date(2026, 11, 12, 18, 30, 0, 0, time.UTC)
	reflected := time.Date(2026, 11, 13, 9, 15, 0, 0, time.UTC)
	talked := time.Date(2026, 11, 13, 22, 58, 0, 0, time.UTC)
	due := talked.Add(30 * time.Minute)

	memories := []db.RoomMemory{
		{ID: 1, RoomID: "!eve:example.com", Memory: "Several people here play EVE Online.", CreatedAt: early, UpdatedAt: early, CreatedTraceID: ptr("aaa"), UpdatedTraceID: ptr("aaa")},
		{ID: 2, RoomID: "!eve:example.com", Memory: "People here ask for answers in Finnish.", CreatedAt: early, UpdatedAt: late, CreatedTraceID: ptr("aaa"), UpdatedTraceID: ptr("bbb")},
		{ID: 3, RoomID: "!hq:example.com", Memory: "The room plans a sauna evening every December.", CreatedAt: early, UpdatedAt: early},
	}
	states := []db.RoomMemoryState{
		{RoomID: "!eve:example.com", After: 10, Until: 10, ReflectedAt: &reflected, LastAttemptAt: &reflected, LastOutcome: ptr("changed"), LastTraceID: ptr("bbb")},
		{RoomID: "!hq:example.com", After: 4, Until: 4},
		{RoomID: "!new:example.com", After: 0, Until: 7, LatestAt: &talked},
		{RoomID: "!dm:example.com", After: 0, Until: 3, LatestAt: &talked},
	}

	lookups := make(map[string]int)
	entries := roomMemoryEntries(memories, states,
		func(state db.RoomMemoryState) chat.ReflectionSchedule {
			if state.HasNewHistory() {
				return chat.ReflectionSchedule{Status: chat.ScheduleWaiting, DueAt: &due}
			}
			return chat.ReflectionSchedule{Status: chat.ScheduleUpToDate}
		},
		func(roomID string) string {
			lookups[roomID]++
			return map[string]string{"!eve:example.com": "EVE corp", "!hq:example.com": "Siika HQ"}[roomID]
		},
		func(roomID string) bool { return roomID == "!dm:example.com" },
	)

	if len(entries) != 3 {
		t.Fatalf("got %d rooms, want 3: %#v", len(entries), entries)
	}
	pending, eve, hq := entries[0], entries[1], entries[2]

	if pending.RoomID != "!new:example.com" || len(pending.Memories) != 0 || pending.Memories == nil {
		t.Errorf("the room with new conversations is %#v", pending)
	}
	if pending.Status.State != chat.ScheduleWaiting || *pending.Status.DueAt != "2026-11-13T23:28:00Z" ||
		*pending.Status.LatestActivityAt != "2026-11-13T22:58:00Z" || pending.Status.LastOutcome != nil {
		t.Errorf("the room with new conversations stands as %#v", pending.Status)
	}

	if eve.RoomName != "EVE corp" || len(eve.Memories) != 2 || *eve.LastReflectedAt != "2026-11-13T09:15:00Z" {
		t.Errorf("the room thought back over most recently is %#v", eve)
	}
	if eve.Status.State != chat.ScheduleUpToDate || eve.Status.DueAt != nil || *eve.Status.LastOutcome != "changed" ||
		*eve.Status.LastTraceID != "bbb" || *eve.Status.LastAttemptAt != "2026-11-13T09:15:00Z" {
		t.Errorf("the room thought back over most recently stands as %#v", eve.Status)
	}
	if got := eve.Memories[1]; got.UpdatedAt != "2026-11-12T18:30:00Z" || *got.CreatedTraceID != "aaa" || *got.UpdatedTraceID != "bbb" {
		t.Errorf("the revised memory reads as %#v", got)
	}

	if hq.RoomName != "Siika HQ" || len(hq.Memories) != 1 || hq.LastReflectedAt != nil || hq.Memories[0].CreatedTraceID != nil {
		t.Errorf("the room with only an old memory is %#v", hq)
	}

	for roomID, count := range lookups {
		if count != 1 {
			t.Errorf("the name of %s was looked up %d times, want once", roomID, count)
		}
	}
	if lookups["!dm:example.com"] != 0 {
		t.Error("the name of a direct chat was looked up")
	}

	encoded, err := json.Marshal(RoomMemoriesResponse{Rooms: entries})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"last_reflected_at":null`, `"created_trace_id":null`, `"last_outcome":null`, `"memories":[]`} {
		if !strings.Contains(string(encoded), want) {
			t.Errorf("the response lacks %s: %s", want, encoded)
		}
	}
}

func TestRoomMemoryEntriesWithoutRooms(t *testing.T) {
	entries := roomMemoryEntries(nil, nil, nil, nil, nil)
	if entries == nil || len(entries) != 0 {
		t.Errorf("roomMemoryEntries(nil) = %#v, want an empty list", entries)
	}
}

func TestReflectResponse(t *testing.T) {
	response := reflectResponse(chat.ReflectionResult{
		Outcome: "changed", Conversations: 4, Shown: 3, Added: 1, Updated: 2, Rejected: 1, TraceID: "abc",
	})
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"outcome":"changed","skipped":false,"failed":false,"conversation_count":4,"shown_conversation_count":3,` +
		`"added":1,"updated":2,"deleted":0,"rejected":1,"trace_id":"abc"}`
	if string(encoded) != want {
		t.Errorf("the response encodes as\n%s\nwant\n%s", encoded, want)
	}
}

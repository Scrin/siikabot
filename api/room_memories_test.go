package api

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Scrin/siikabot/db"
)

// The memories are grouped by room, each room's name is looked up once, and the room whose memories
// changed most recently comes first
func TestRoomMemoryEntries(t *testing.T) {
	early := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	late := time.Date(2026, 11, 12, 18, 30, 0, 0, time.UTC)
	reflected := time.Date(2026, 11, 13, 9, 15, 0, 0, time.UTC)

	lookups := make(map[string]int)
	entries := roomMemoryEntries([]db.RoomMemory{
		{ID: 1, RoomID: "!eve:example.com", Memory: "Several people here play EVE Online.", CreatedAt: early, UpdatedAt: early},
		{ID: 2, RoomID: "!eve:example.com", Memory: "People here ask for answers in Finnish.", CreatedAt: early, UpdatedAt: early},
		{ID: 3, RoomID: "!hq:example.com", Memory: "The room plans a sauna evening every December.", CreatedAt: early, UpdatedAt: late},
	}, []db.RoomMemoryProgress{
		{RoomID: "!hq:example.com", ReflectedAt: &reflected},
		{RoomID: "!eve:example.com"},
		{RoomID: "!dm:example.com", ReflectedAt: &reflected},
	}, func(roomID string) string {
		lookups[roomID]++
		return map[string]string{"!eve:example.com": "EVE corp", "!hq:example.com": "Siika HQ"}[roomID]
	})

	if len(entries) != 2 {
		t.Fatalf("got %d rooms, want 2: %#v", len(entries), entries)
	}
	hq, eve := entries[0], entries[1]
	if hq.RoomID != "!hq:example.com" || hq.RoomName != "Siika HQ" || len(hq.Memories) != 1 {
		t.Errorf("the most recently changed room is %#v", hq)
	}
	if hq.LastReflectedAt == nil || *hq.LastReflectedAt != "2026-11-13T09:15:00Z" {
		t.Errorf("the room was last thought back over at %v", hq.LastReflectedAt)
	}
	if got := hq.Memories[0]; got.CreatedAt != "2026-11-01T12:00:00Z" || got.UpdatedAt != "2026-11-12T18:30:00Z" {
		t.Errorf("the revised memory reads as %#v", got)
	}
	if eve.RoomName != "EVE corp" || len(eve.Memories) != 2 || eve.Memories[0].ID != 1 || eve.LastReflectedAt != nil {
		t.Errorf("the other room is %#v", eve)
	}
	for roomID, count := range lookups {
		if count != 1 {
			t.Errorf("the name of %s was looked up %d times, want once", roomID, count)
		}
	}
	if lookups["!dm:example.com"] != 0 {
		t.Error("the name of a room without memories was looked up")
	}

	encoded, err := json.Marshal(RoomMemoriesResponse{Rooms: entries})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"last_reflected_at":null`) {
		t.Errorf("a room never thought back over encodes as %s", encoded)
	}
}

func TestRoomMemoryEntriesWithoutMemories(t *testing.T) {
	entries := roomMemoryEntries(nil, nil, nil)
	if entries == nil || len(entries) != 0 {
		t.Errorf("roomMemoryEntries(nil) = %#v, want an empty list", entries)
	}
}

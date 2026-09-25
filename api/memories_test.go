package api

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Scrin/siikabot/db"
)

func TestMemoryResponsesSayWhereEachMemoryApplies(t *testing.T) {
	group := "!group:example.com"
	created := time.Date(2026, 9, 25, 14, 2, 0, 0, time.UTC)
	memories := []db.UserMemory{
		{ID: 1, Memory: "said in a DM", CreatedAt: created},
		{ID: 2, Memory: "said in the group", RoomID: &group, CreatedAt: created},
		{ID: 3, Memory: "also said in the group", RoomID: &group, CreatedAt: created},
	}

	lookups := 0
	responses := memoryResponses(memories, func(roomID string) string {
		lookups++
		if roomID != group {
			t.Errorf("looked up %q", roomID)
		}
		return "Siika HQ"
	})

	if responses[0].RoomID != nil || responses[0].RoomName != "" {
		t.Errorf("a DM memory reads as %#v, want no room", responses[0])
	}
	for _, response := range responses[1:] {
		if response.RoomID == nil || *response.RoomID != group || response.RoomName != "Siika HQ" {
			t.Errorf("a group memory reads as %#v, want its room and the room's name", response)
		}
	}
	if lookups != 1 {
		t.Errorf("the room's name was looked up %d times, want once", lookups)
	}
}

// A DM memory says so with an explicit null, rather than by leaving the field out
func TestMemoryResponseEncodesADMMemoryAsNull(t *testing.T) {
	encoded, err := json.Marshal(memoryResponses([]db.UserMemory{{ID: 1, Memory: "said in a DM"}}, nil)[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"room_id":null`) || strings.Contains(string(encoded), "room_name") {
		t.Errorf("a DM memory encodes as %s", encoded)
	}
}

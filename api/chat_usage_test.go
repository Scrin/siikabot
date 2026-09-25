package api

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Scrin/siikabot/db"
)

// A turn the bot stayed silent on is counted apart from the failures: it went as it should
func TestUsageEntriesCountSilentTurns(t *testing.T) {
	entries, totals := usageEntries([]db.ChatUsageSummary{
		{RoomID: "!a:example.com", Model: "text", Turns: 10, PromptTokens: 1000, CachedPromptTokens: 500, Failures: 1, Silent: 2},
		{RoomID: "!b:example.com", Model: "text", Turns: 5, PromptTokens: 1000, Failures: 0, Silent: 1},
	}, func(roomID string) string { return "Room " + roomID[1:2] })

	if len(entries) != 2 || entries[0].Silent != 2 || entries[0].Failures != 1 || entries[0].RoomName != "Room a" {
		t.Errorf("entries = %#v", entries)
	}
	if totals.Turns != 15 || totals.Failures != 1 || totals.Silent != 3 || totals.CacheHitRate != 0.25 {
		t.Errorf("totals = %#v, want 15 turns, 1 failure, 3 silent and a 25%% cache hit rate", totals)
	}

	encoded, err := json.Marshal(totals)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"failures":1,"silent":3`) {
		t.Errorf("totals encode as %s", encoded)
	}
}

func TestUsageEntriesWithoutUsage(t *testing.T) {
	entries, totals := usageEntries(nil, nil)
	if entries == nil || len(entries) != 0 || totals.Turns != 0 || totals.Model != "all" {
		t.Errorf("usageEntries(nil) = %#v, %#v", entries, totals)
	}
}

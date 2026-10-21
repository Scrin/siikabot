package api

import (
	"encoding/json"
	"maps"
	"strings"
	"testing"

	"github.com/Scrin/siikabot/db"
)

// A turn the bot stayed silent on is counted apart from the failures: it went as it should
func TestUsageEntriesCountSilentTurns(t *testing.T) {
	entries, totals := usageEntries([]db.ChatUsageSummary{
		{RoomID: "!a:example.com", Model: "text", Turns: 10, PromptTokens: 1000, CachedPromptTokens: 500, Failures: 1, Silent: 2},
		{RoomID: "!b:example.com", Model: "text", Turns: 5, PromptTokens: 1000, Failures: 0, Silent: 1},
	}, nil, func(roomID string) string { return "Room " + roomID[1:2] })

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
	entries, totals := usageEntries(nil, nil, nil)
	if entries == nil || len(entries) != 0 || totals.Turns != 0 || totals.Model != "all" || totals.FailuresByOutcome == nil {
		t.Errorf("usageEntries(nil) = %#v, %#v", entries, totals)
	}
}

// The failures of each room and model are broken down by how its turns failed, and the totals add
// the breakdowns up, so the dashboard can say which limit the turns ran into
func TestUsageEntriesBreakFailuresDown(t *testing.T) {
	entries, totals := usageEntries([]db.ChatUsageSummary{
		{RoomID: "!a:example.com", Model: "first", Turns: 10, Failures: 3},
		{RoomID: "!a:example.com", Model: "second", Turns: 4, Failures: 1},
		{RoomID: "!b:example.com", Model: "first", Turns: 5},
	}, []db.ChatUsageFailures{
		{RoomID: "!a:example.com", Model: "first", Outcome: "turn_timeout", Turns: 2},
		{RoomID: "!a:example.com", Model: "first", Outcome: "tool_limit", Turns: 1},
		{RoomID: "!a:example.com", Model: "second", Outcome: "turn_timeout", Turns: 1},
	}, func(string) string { return "" })

	for i, want := range []map[string]int{
		{"turn_timeout": 2, "tool_limit": 1},
		{"turn_timeout": 1},
		{},
	} {
		if got := entries[i].FailuresByOutcome; !maps.Equal(got, want) {
			t.Errorf("entry %d failed by %v, want %v", i, got, want)
		}
	}
	if got, want := totals.FailuresByOutcome, map[string]int{"turn_timeout": 3, "tool_limit": 1}; !maps.Equal(got, want) {
		t.Errorf("the totals failed by %v, want %v", got, want)
	}

	// An entry without failures has an empty breakdown rather than none
	encoded, err := json.Marshal(entries[2])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"failures_by_outcome":{}`) {
		t.Errorf("an entry without failures encodes as %s", encoded)
	}
}

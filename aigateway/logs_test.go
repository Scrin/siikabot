package aigateway

import (
	"encoding/json"
	"testing"
	"time"
)

// TestGatewayLogParsing verifies a real logs API payload parses, including the optional cost
// field and the non-sortable hash ids.
func TestGatewayLogParsing(t *testing.T) {
	body := []byte(`{
		"success": true,
		"errors": [],
		"result": [
			{
				"id": "386e367f5df9fc5f06d759672c80d6601a641236830982985b6895c16d7b7ed6",
				"created_at": "2026-08-05T14:44:36.727Z",
				"model": "openai/gpt-4o-mini",
				"provider": "openai",
				"cost": 0.000027449999999999996,
				"custom_cost": false,
				"tokens_in": 87,
				"tokens_out": 24,
				"duration": 824,
				"cached": false,
				"success": true,
				"status_code": 200
			},
			{
				"id": "a1ce1c8691dd1ace6a8ce1c439505b6938fe5ef823042c70cff5786de351169e",
				"created_at": "2026-08-05T14:44:40.346Z",
				"model": "openai/definitely-not-a-real-model",
				"provider": "unknown",
				"tokens_in": 0,
				"tokens_out": 0,
				"duration": 0,
				"cached": false,
				"success": false,
				"status_code": 500
			}
		]
	}`)

	var resp logsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.Result) != 2 {
		t.Fatalf("expected 2 logs, got %d", len(resp.Result))
	}

	first := resp.Result[0]
	if first.Cost == nil {
		t.Fatal("expected cost to be populated")
	}
	if *first.Cost <= 0 {
		t.Errorf("expected a positive cost, got %v", *first.Cost)
	}
	if first.TokensIn != 87 || first.TokensOut != 24 {
		t.Errorf("unexpected tokens: in=%d out=%d", first.TokensIn, first.TokensOut)
	}
	if first.Duration != 824 {
		t.Errorf("expected duration 824, got %d", first.Duration)
	}
	if first.CreatedAt.IsZero() {
		t.Error("expected created_at to be parsed")
	}

	// An absent cost must stay nil rather than becoming a spurious zero-cost sample
	if resp.Result[1].Cost != nil {
		t.Errorf("expected a nil cost when absent, got %v", *resp.Result[1].Cost)
	}
	if resp.Result[1].Success {
		t.Error("expected the second entry to be a failure")
	}
}

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t.Fatalf("bad time %q: %v", s, err)
	}
	return parsed
}

// logsNewestFirst mirrors how the API orders results.
func logsNewestFirst(t *testing.T, entries ...GatewayLog) []GatewayLog {
	t.Helper()
	return entries
}

// TestCollectFreshLogsStopsAtCursor verifies only entries newer than the cursor are collected.
func TestCollectFreshLogsStopsAtCursor(t *testing.T) {
	cursor := mustTime(t, "2026-08-05T14:44:38.000Z")

	all := logsNewestFirst(t,
		GatewayLog{ID: "c", CreatedAt: mustTime(t, "2026-08-05T14:44:40.000Z")},
		GatewayLog{ID: "b", CreatedAt: mustTime(t, "2026-08-05T14:44:39.000Z")},
		GatewayLog{ID: "a", CreatedAt: mustTime(t, "2026-08-05T14:44:37.000Z")}, // older than cursor
	)

	var fresh []GatewayLog
	for _, entry := range all {
		if entry.CreatedAt.Before(cursor) {
			break
		}
		fresh = append(fresh, entry)
	}

	if len(fresh) != 2 {
		t.Fatalf("expected 2 fresh entries, got %d", len(fresh))
	}
	if fresh[0].ID != "c" || fresh[1].ID != "b" {
		t.Errorf("unexpected entries: %s, %s", fresh[0].ID, fresh[1].ID)
	}
}

// TestCursorAdvanceWithSharedTimestamps covers the reason the cursor tracks ids as well as a
// timestamp: created_at is not unique, and log ids are content hashes rather than sortable
// values, so a shared timestamp would otherwise be re-recorded or skipped.
func TestCursorAdvanceWithSharedTimestamps(t *testing.T) {
	shared := mustTime(t, "2026-08-05T14:44:40.346Z")
	older := mustTime(t, "2026-08-05T14:44:36.727Z")

	fresh := []GatewayLog{
		{ID: "newest_1", CreatedAt: shared},
		{ID: "newest_2", CreatedAt: shared},
		{ID: "older_1", CreatedAt: older},
	}

	newCursor := fresh[0].CreatedAt
	newSeen := make(map[string]bool)
	for _, entry := range fresh {
		if entry.CreatedAt.Equal(newCursor) {
			newSeen[entry.ID] = true
		}
	}

	if !newCursor.Equal(shared) {
		t.Errorf("expected the cursor to advance to the newest timestamp, got %v", newCursor)
	}
	if len(newSeen) != 2 || !newSeen["newest_1"] || !newSeen["newest_2"] {
		t.Errorf("expected both ids at the cursor timestamp to be remembered, got %v", newSeen)
	}
	if newSeen["older_1"] {
		t.Error("older entries should not be tracked in the cursor id set")
	}

	// On the next poll the same two entries must be skipped, and a third one at the same
	// timestamp must still be picked up.
	next := []GatewayLog{
		{ID: "newest_3", CreatedAt: shared},
		{ID: "newest_2", CreatedAt: shared},
		{ID: "newest_1", CreatedAt: shared},
	}

	var picked []string
	for _, entry := range next {
		if entry.CreatedAt.Before(newCursor) {
			break
		}
		if entry.CreatedAt.Equal(newCursor) && newSeen[entry.ID] {
			continue
		}
		picked = append(picked, entry.ID)
	}

	if len(picked) != 1 || picked[0] != "newest_3" {
		t.Errorf("expected only newest_3 to be picked up, got %v", picked)
	}
}

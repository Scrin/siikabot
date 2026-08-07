package chat

import (
	"strings"
	"testing"
	"time"

	"github.com/Scrin/siikabot/db"
)

// sizedText produces a message whose estimated cost is approximately the requested token count
func sizedText(tokens int) string {
	return strings.Repeat("x", (tokens-messageTokenOverhead)*4)
}

// userRow builds a turn-boundary row of approximately the given token size
func userRow(id int64, tokens int) db.ChatMessage {
	return db.ChatMessage{ID: id, Role: "user", MessageType: "text", Message: sizedText(tokens)}
}

// assistantRow builds a non-boundary row of approximately the given token size
func assistantRow(id int64, tokens int) db.ChatMessage {
	return db.ChatMessage{ID: id, Role: "assistant", MessageType: "text", Message: sizedText(tokens)}
}

func TestEstimateTokensGrowsWithLength(t *testing.T) {
	short := estimateTokens("hello")
	long := estimateTokens(strings.Repeat("hello ", 100))

	if short >= long {
		t.Errorf("expected longer text to estimate higher, got %d and %d", short, long)
	}
	if estimateTokens("") != messageTokenOverhead {
		t.Errorf("expected an empty message to cost only the overhead, got %d", estimateTokens(""))
	}
}

// TestTrimToLowMarkStopsAtTurnBoundary verifies the window is cut at a user message. Cutting
// mid-turn would strand a tool call without its response, which the chat API rejects.
func TestTrimToLowMarkStopsAtTurnBoundary(t *testing.T) {
	history := []db.ChatMessage{
		userRow(1, 100),
		assistantRow(2, 100),
		userRow(3, 100),
		assistantRow(4, 100),
		userRow(5, 100),
		assistantRow(6, 100),
	}

	trimmed, anchorID := trimToLowMark(history, 250)

	if anchorID == 0 {
		t.Fatal("expected an anchor to be chosen")
	}
	if !isTurnBoundary(trimmed[0]) {
		t.Errorf("window starts at a non-boundary row: role=%q type=%q", trimmed[0].Role, trimmed[0].MessageType)
	}
	if trimmed[0].ID != anchorID {
		t.Errorf("anchor id %d does not match the first row %d", anchorID, trimmed[0].ID)
	}
	if got := estimateHistoryTokens(trimmed); got > 250 {
		t.Errorf("trimmed window is %d tokens, above the low mark of 250", got)
	}
}

// TestTrimToLowMarkKeepsTheMostRecentTurns verifies trimming drops from the old end, not the new one
func TestTrimToLowMarkKeepsTheMostRecentTurns(t *testing.T) {
	history := []db.ChatMessage{
		userRow(1, 100),
		userRow(2, 100),
		userRow(3, 100),
		userRow(4, 100),
	}

	trimmed, _ := trimToLowMark(history, 250)

	if trimmed[len(trimmed)-1].ID != 4 {
		t.Errorf("expected the newest row to be kept, last row is id %d", trimmed[len(trimmed)-1].ID)
	}
	if trimmed[0].ID == 1 {
		t.Error("expected the oldest row to be dropped")
	}
}

// TestTrimToLowMarkFallsBackToNewestBoundary covers a single turn already larger than the low mark:
// keeping the most recent turn beats returning nothing usable.
func TestTrimToLowMarkFallsBackToNewestBoundary(t *testing.T) {
	history := []db.ChatMessage{
		userRow(1, 500),
		userRow(2, 500),
	}

	trimmed, anchorID := trimToLowMark(history, 100)

	if anchorID != 2 {
		t.Errorf("expected to anchor at the newest boundary (2), got %d", anchorID)
	}
	if len(trimmed) != 1 || trimmed[0].ID != 2 {
		t.Errorf("expected only the newest turn to be kept, got %d rows", len(trimmed))
	}
}

// TestTrimToLowMarkWithNoBoundary verifies a window with no user message refuses to advance rather
// than cutting at an arbitrary point
func TestTrimToLowMarkWithNoBoundary(t *testing.T) {
	history := []db.ChatMessage{
		assistantRow(1, 500),
		assistantRow(2, 500),
	}

	trimmed, anchorID := trimToLowMark(history, 100)

	if anchorID != 0 {
		t.Errorf("expected no anchor when there is no turn boundary, got %d", anchorID)
	}
	if len(trimmed) != len(history) {
		t.Error("expected the history to be returned untouched when no anchor is possible")
	}
}

// TestWindowIsStableBetweenEvictions is the point of the whole design: the window must not change
// on every turn, because a prefix that shifts constantly cannot be cached and unsettles the model.
func TestWindowIsStableBetweenEvictions(t *testing.T) {
	const high, low = 1000, 500

	history := []db.ChatMessage{userRow(1, 100), userRow(2, 100), userRow(3, 100)}

	// Grow the conversation one turn at a time, trimming only when over the high mark, and record
	// the first row of the window after each turn
	var firstRowIDs []int64
	evictions := 0
	for id := int64(4); id <= 20; id++ {
		history = append(history, userRow(id, 100))
		if estimateHistoryTokens(history) > high {
			trimmed, anchorID := trimToLowMark(history, low)
			if anchorID == 0 {
				t.Fatal("expected an anchor while trimming")
			}
			history = trimmed
			evictions++
		}
		firstRowIDs = append(firstRowIDs, history[0].ID)
	}

	if evictions == 0 {
		t.Fatal("expected at least one eviction over the run")
	}

	// The first row changes only when an eviction happens, so most turns leave it untouched
	changes := 0
	for i := 1; i < len(firstRowIDs); i++ {
		if firstRowIDs[i] != firstRowIDs[i-1] {
			changes++
		}
	}

	if changes != evictions {
		t.Errorf("window start changed %d times but only %d evictions occurred", changes, evictions)
	}
	if changes >= len(firstRowIDs)-1 {
		t.Errorf("window start changed on nearly every turn (%d of %d), which is the behaviour this replaces",
			changes, len(firstRowIDs)-1)
	}
}

// TestReplayableToolResponseExpiry is the E2 test: an expired result is replaced by a marker rather
// than deleted, so the conversation structure survives.
func TestReplayableToolResponseExpiry(t *testing.T) {
	now := time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)
	toolName := "get_weather"

	fresh := db.ChatMessage{Message: "-7 C", ToolName: &toolName, Expiry: &future}
	if got := replayableToolResponse(fresh, now); got != "-7 C" {
		t.Errorf("expected an unexpired result to be replayed verbatim, got %q", got)
	}

	noExpiry := db.ChatMessage{Message: "-7 C", ToolName: &toolName}
	if got := replayableToolResponse(noExpiry, now); got != "-7 C" {
		t.Errorf("expected a result without an expiry to be replayed verbatim, got %q", got)
	}

	stale := db.ChatMessage{Message: "-7 C", ToolName: &toolName, Expiry: &past}
	got := replayableToolResponse(stale, now)
	if got == "-7 C" {
		t.Error("expected an expired result to be replaced")
	}
	if !strings.Contains(got, "expired") {
		t.Errorf("expected the marker to say the result expired, got %q", got)
	}
	if !strings.Contains(got, toolName) {
		t.Errorf("expected the marker to name the tool, got %q", got)
	}
	if got == "" {
		t.Error("expected a non-empty marker so the tool reply is not left blank")
	}
}

// TestReplayableToolResponseWithoutToolName verifies a missing tool name does not produce a broken
// marker, since ToolName is nullable in the schema
func TestReplayableToolResponseWithoutToolName(t *testing.T) {
	now := time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)
	past := now.Add(-time.Hour)

	got := replayableToolResponse(db.ChatMessage{Message: "data", Expiry: &past}, now)

	if !strings.Contains(got, "expired") {
		t.Errorf("expected an expiry marker, got %q", got)
	}
}

// TestIsTurnBoundary verifies only user text messages start a turn
func TestIsTurnBoundary(t *testing.T) {
	cases := []struct {
		name string
		msg  db.ChatMessage
		want bool
	}{
		{"user text", db.ChatMessage{Role: "user", MessageType: "text"}, true},
		{"legacy user row", db.ChatMessage{Role: "user", MessageType: ""}, true},
		{"assistant text", db.ChatMessage{Role: "assistant", MessageType: "text"}, false},
		{"tool call", db.ChatMessage{Role: "assistant", MessageType: "tool_call"}, false},
		{"tool response", db.ChatMessage{Role: "tool", MessageType: "tool_response"}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isTurnBoundary(tc.msg); got != tc.want {
				t.Errorf("isTurnBoundary(%s) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

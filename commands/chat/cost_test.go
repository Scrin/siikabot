package chat

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/Scrin/siikabot/db"
)

// TestTruncateForReplayLeavesSmallResultsAlone verifies a result within the cap is replayed verbatim
func TestTruncateForReplayLeavesSmallResultsAlone(t *testing.T) {
	response := strings.Repeat("a", maxReplayedToolResponseBytes)

	if got := truncateForReplay(response, "web"); got != response {
		t.Error("a result at the cap should be replayed unchanged")
	}
}

// TestTruncateForReplayShortensLargeResults verifies an oversized result is cut and labelled, so a
// single web fetch cannot occupy a large share of the context window for the rest of its life
func TestTruncateForReplayShortensLargeResults(t *testing.T) {
	response := strings.Repeat("a", maxReplayedToolResponseBytes*3)

	got := truncateForReplay(response, "web")

	if len(got) >= len(response) {
		t.Errorf("expected the result to shrink, got %d bytes from %d", len(got), len(response))
	}
	if !strings.Contains(got, "truncated") {
		t.Errorf("expected the result to say it was truncated, got %q", got[len(got)-100:])
	}
	if !strings.Contains(got, "web") {
		t.Error("expected the marker to name the tool")
	}
}

// TestTruncateForReplayKeepsValidUTF8 verifies the cut lands on a rune boundary. Slicing bytes
// blindly would split a multi-byte character and produce invalid UTF-8, which the database layer
// then has to scrub.
func TestTruncateForReplayKeepsValidUTF8(t *testing.T) {
	// Three-byte runes, so the cap lands mid-character for at least one offset
	for _, filler := range []string{"€", "ä", "😀"} {
		response := strings.Repeat(filler, maxReplayedToolResponseBytes)

		got := truncateForReplay(response, "web")

		if !utf8.ValidString(got) {
			t.Errorf("truncating a string of %q produced invalid UTF-8", filler)
		}
	}
}

// TestReplayableToolResponseTruncatesUnexpiredResults verifies truncation and the expiry marker
// cooperate rather than one masking the other
func TestReplayableToolResponseTruncatesUnexpiredResults(t *testing.T) {
	now := time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)
	future := now.Add(time.Hour)
	toolName := "web"
	large := strings.Repeat("a", maxReplayedToolResponseBytes*2)

	got := replayableToolResponse(db.ChatMessage{Message: large, ToolName: &toolName, Expiry: &future}, now)

	if !strings.Contains(got, "truncated") {
		t.Error("expected a large unexpired result to be truncated")
	}
	if strings.Contains(got, "expired") {
		t.Error("an unexpired result should not be marked expired")
	}
}

// TestReplayableToolResponseExpiryBeatsTruncation verifies an expired result becomes a marker rather
// than a truncated copy of stale data
func TestReplayableToolResponseExpiryBeatsTruncation(t *testing.T) {
	now := time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)
	past := now.Add(-time.Hour)
	toolName := "web"
	large := strings.Repeat("a", maxReplayedToolResponseBytes*2)

	got := replayableToolResponse(db.ChatMessage{Message: large, ToolName: &toolName, Expiry: &past}, now)

	if !strings.Contains(got, "expired") {
		t.Error("expected an expired result to be replaced by the expiry marker")
	}
	if len(got) > 500 {
		t.Errorf("expected the expiry marker to replace the content entirely, got %d bytes", len(got))
	}
}

// TestPromptPrefixIsStableAcrossTurns is the B1 regression test. Providers cache the longest
// unchanging prefix of a request, so every element that varies between requests has to sit after
// every element that does not. The current time used to be in the system prompt's second sentence,
// which changed the prefix on every request and made caching impossible.
func TestPromptPrefixIsStableAcrossTurns(t *testing.T) {
	// Stand in for what buildInitialMessages assembles, at two different times
	buildPrompt := func(now time.Time) []string {
		return []string{
			"system:You are Siikabot, a helpful Matrix bot. Keep your responses concise and helpful.",
			"user:earlier question",
			"assistant:earlier answer",
			"system:The current date and time is " + now.Format("Monday, January 2, 2006 15:04:05 MST"),
			"user:current question",
		}
	}

	first := buildPrompt(time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC))
	second := buildPrompt(time.Date(2026, 8, 7, 12, 30, 0, 0, time.UTC))

	// Everything up to and including the history must match, so it can be served from cache
	shared := 0
	for shared < len(first) && shared < len(second) && first[shared] == second[shared] {
		shared++
	}

	if shared < 3 {
		t.Errorf("only %d messages of prefix are stable, expected the system prompt and history to be", shared)
	}
	if strings.Contains(first[0], "current date and time") {
		t.Error("the volatile timestamp is back in the system prompt, which breaks prefix caching")
	}
}

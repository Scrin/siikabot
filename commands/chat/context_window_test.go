package chat

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Scrin/siikabot/aigateway"
	"github.com/Scrin/siikabot/db"
)

// sizedText produces a message whose estimated cost is approximately the requested token count
func sizedText(tokens int) string {
	return strings.Repeat("x", (tokens-messageTokenOverhead)*4)
}

// userRow builds a turn-boundary row of approximately the given token size. The size is that of the
// rendered message, header included, since that is what the budget measures.
func userRow(id int64, tokens int) db.ChatMessage {
	row := textMsg("user", "", time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC))
	row.ID = id
	header := len(replayText(row, testNow))
	row.Message = strings.Repeat("x", max(0, (tokens-messageTokenOverhead)*4-header))
	return row
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

	trimmed, anchorID := trimToLowMark(history, 250, testNow)

	if anchorID == 0 {
		t.Fatal("expected an anchor to be chosen")
	}
	if !isTurnBoundary(trimmed[0]) {
		t.Errorf("window starts at a non-boundary row: role=%q type=%q", trimmed[0].Role, trimmed[0].MessageType)
	}
	if trimmed[0].ID != anchorID {
		t.Errorf("anchor id %d does not match the first row %d", anchorID, trimmed[0].ID)
	}
	if got := estimateHistoryTokens(trimmed, testNow); got > 250 {
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

	trimmed, _ := trimToLowMark(history, 250, testNow)

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

	trimmed, anchorID := trimToLowMark(history, 100, testNow)

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

	trimmed, anchorID := trimToLowMark(history, 100, testNow)

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
		if estimateHistoryTokens(history, testNow) > high {
			trimmed, anchorID := trimToLowMark(history, low, testNow)
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

// TestIsTurnBoundary verifies only user text messages start a turn
func TestIsTurnBoundary(t *testing.T) {
	cases := []struct {
		name string
		msg  db.ChatMessage
		want bool
	}{
		{"user text", db.ChatMessage{Role: "user", MessageType: "text"}, true},
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

// TestRowCapNeverCutsAWindowWithinBudget is the property the row cap is derived for: a read the cap
// cuts short is over the high mark already, so the cap can never decide the window in the budget's
// place. Rows of the smallest possible size are the hardest case, since the most of them fit.
func TestRowCapNeverCutsAWindowWithinBudget(t *testing.T) {
	smallest := db.ChatMessage{Role: "assistant", MessageType: "text"}
	if got := estimateRowTokens(smallest, testNow); got != messageTokenOverhead {
		t.Fatalf("an empty row is estimated at %d tokens, want %d", got, messageTokenOverhead)
	}

	for _, high := range []int{1, 3, 4, 5, 16384, 100000, 102401} {
		cut := slices.Repeat([]db.ChatMessage{smallest}, maxContextRows(high))
		if got := estimateHistoryTokens(cut, testNow); got <= high {
			t.Errorf("a read cut at the cap of %d rows is %d tokens, within the high mark of %d",
				maxContextRows(high), got, high)
		}
	}
}

// TestBudgetMeasuresToolResultsAsReplayed verifies the budget counts a tool result as the model sees
// it: cut short while it is current, and as its expiry marker once it has expired. Counting results
// as stored trimmed the windows that held them long before they reached the budget.
func TestBudgetMeasuresToolResultsAsReplayed(t *testing.T) {
	past, future := testNow.Add(-time.Hour), testNow.Add(time.Hour)

	long := responseMsg("call_1", "get_web_content", strings.Repeat("a", maxReplayedToolResponseBytes*3), testNow)
	long.Expiry = &future
	expired := responseMsg("call_2", "get_fmi_data", strings.Repeat("b", 2000), testNow)
	expired.Expiry = &past

	history := []db.ChatMessage{
		textMsg("user", "read this page and check the weather", testNow),
		callMsg("call_1", "get_web_content", `{"url":"https://example.com"}`, testNow),
		long,
		callMsg("call_2", "get_fmi_data", `{"place":"Oulu"}`, testNow),
		expired,
		textMsg("assistant", "here you go", testNow),
	}

	// The budget has to measure exactly the text the replay produces
	var messages []aigateway.Message
	processHistoryMessages(context.Background(), history, "low", testNow, &messages)
	replayed := make(map[string]string)
	for _, msg := range messages {
		if msg.Role == "tool" {
			replayed[msg.ToolCallID], _ = msg.Content.(string)
		}
	}
	for _, row := range []db.ChatMessage{long, expired} {
		if got, want := replayText(row, testNow), replayed[*row.ToolCallID]; got != want {
			t.Errorf("the %s result is measured as %d bytes of text, but replayed as %d", *row.ToolName, len(got), len(want))
		}
		if got, stored := estimateRowTokens(row, testNow), estimateTokens(row.Message); got >= stored {
			t.Errorf("the %s result is estimated at %d tokens, not less than the %d it is stored as", *row.ToolName, got, stored)
		}
	}

	// Expiring only ever makes a result cheaper, so the passing of time alone never pushes a window
	// over the high mark
	if before, after := estimateRowTokens(long, future.Add(-time.Minute)), estimateRowTokens(long, future); after >= before {
		t.Errorf("the result is estimated at %d tokens once it has expired, not less than the %d before", after, before)
	}
}

func TestWindowHasTurn(t *testing.T) {
	history := []db.ChatMessage{{TurnEventID: "$question"}, {TurnEventID: "$question"}, {TurnEventID: "$thread"}}

	if !windowHasTurn(history, "$question") {
		t.Error("a turn in the window wasn't found")
	}
	if windowHasTurn(history, "$other") || windowHasTurn(history, "") {
		t.Error("a turn that isn't in the window was found")
	}
}

// imageTurnRow builds a turn-boundary row of a message with an image of its own, kept with it
func imageTurnRow(id int64, width, height int) db.ChatMessage {
	row := userRow(id, 50)
	row.TurnEventID = fmt.Sprintf("$turn%d", id)
	hasImage := true
	row.HasImage = &hasImage
	row.Images = []db.ChatImage{{EventID: row.TurnEventID, MIMEType: "image/jpeg", Width: width, Height: height}}
	return row
}

func TestEstimateImageTokens(t *testing.T) {
	if got, want := estimateImageTokens(1500, 750), 1500+messageTokenOverhead; got != want {
		t.Errorf("estimateImageTokens(1500, 750) = %d, want %d", got, want)
	}
	if small, large := estimateImageTokens(100, 100), estimateImageTokens(maxImageSide, maxImageSide); small >= large {
		t.Errorf("a small image is estimated at %d tokens and a large one at %d", small, large)
	}
}

// An image counts against the budget for as long as its message is in the window
func TestEstimateHistoryTokensCountsImages(t *testing.T) {
	plain := userRow(1, 100)
	withImage := imageTurnRow(1, 1536, 1152)
	withImage.Message = plain.Message

	if got, least := estimateHistoryTokens([]db.ChatMessage{withImage}, testNow), estimateHistoryTokens([]db.ChatMessage{plain}, testNow)+estimateImageTokens(1536, 1152); got < least {
		t.Errorf("a message with an image is estimated at %d tokens, want at least %d", got, least)
	}
}

// A turn with an image leaves the window like any other, and its image with it
func TestTrimToLowMarkDropsImageTurnsLikeAnyOther(t *testing.T) {
	history := []db.ChatMessage{
		imageTurnRow(1, 1536, 1152),
		assistantRow(2, 100),
		userRow(3, 100),
		assistantRow(4, 100),
	}
	if count, tokens := historyImages(history); count != 1 || tokens != estimateImageTokens(1536, 1152) {
		t.Fatalf("historyImages() = %d, %d, want the one image", count, tokens)
	}

	trimmed, anchorID := trimToLowMark(history, 500, testNow)

	if anchorID != 3 {
		t.Errorf("anchored at %d, want the turn after the image turn (3)", anchorID)
	}
	if count, _ := historyImages(trimmed); count != 0 {
		t.Errorf("%d images left in the trimmed window, want none", count)
	}
}

// The images of a history are those its user messages show, whatever else the rows hold
func TestHistoryImages(t *testing.T) {
	history := []db.ChatMessage{
		imageTurnRow(1, 800, 600),
		assistantRow(2, 100),
		userRow(3, 100),
		imageTurnRow(4, 1536, 1152),
	}

	count, tokens := historyImages(history)
	if want := estimateImageTokens(800, 600) + estimateImageTokens(1536, 1152); count != 2 || tokens != want {
		t.Errorf("historyImages() = %d, %d, want 2, %d", count, tokens, want)
	}
}

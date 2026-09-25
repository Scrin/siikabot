package chat

import (
	"context"

	"github.com/Scrin/siikabot/aigateway"
	"github.com/Scrin/siikabot/db"
	"github.com/Scrin/siikabot/metrics"
	"github.com/rs/zerolog/log"
)

// maxContextRows caps how many history rows are read per turn. The anchor and the token budget
// normally keep the window far below this; it exists so a pathologically stale anchor cannot turn
// into an unbounded read.
const maxContextRows = 1000

// messageTokenOverhead approximates the per-message framing the API adds around the content
const messageTokenOverhead = 4

// estimateTokens approximates the token count of a string.
//
// This is deliberately a heuristic rather than a real tokeniser: an exact count would mean a
// model-specific dependency, and the counts differ between providers anyway. The budget only needs
// to be roughly right, and recordTokenEstimateDrift compares the estimate against the token count
// the API actually reports so the error stays visible.
func estimateTokens(text string) int {
	return len(text)/4 + messageTokenOverhead
}

// estimateHistoryTokens approximates the token cost of replaying the given history rows
func estimateHistoryTokens(history []db.ChatMessage) int {
	total := 0
	for _, msg := range history {
		total += estimateTokens(replayText(msg))
	}
	return total
}

// replayText is the text a history row is replayed as, which is also what its share of the token
// budget is measured on: a user message is replayed with its header
func replayText(msg db.ChatMessage) string {
	if msg.Role == "user" {
		return renderUserTurn(msg.UserTurn(), nil)
	}
	return msg.Message
}

// estimateMessageTokens approximates the prompt size of a built request. Image content parts are
// counted by their text only: a base64 image bears no relation to its token cost, and counting the
// data URI would swamp the estimate with a number that means nothing.
func estimateMessageTokens(messages []aigateway.Message) int {
	total := 0
	for _, msg := range messages {
		switch content := msg.Content.(type) {
		case string:
			total += estimateTokens(content)
		case []aigateway.ContentPart:
			for _, part := range content {
				total += estimateTokens(part.Text)
			}
		default:
			total += messageTokenOverhead
		}
		for _, call := range msg.ToolCalls {
			total += estimateTokens(call.Function.Name) + estimateTokens(call.Function.Arguments)
		}
	}
	return total
}

// currentContextWindow returns a timeline's context window as it currently stands, without
// advancing the anchor. Use this for reporting; buildContextWindow is the one that maintains the
// window. seedTurnEventID is the stored turn a thread was started from, empty for none.
func currentContextWindow(ctx context.Context, timeline db.Timeline, seedTurnEventID string) []db.ChatMessage {
	history, err := db.GetChatHistory(ctx, timeline, seedTurnEventID, maxContextRows)
	if err != nil {
		// Already logged. Continuing without history is better than refusing to answer.
		return nil
	}
	if len(history) == 0 {
		return nil
	}
	if len(history) == maxContextRows {
		log.Warn().Ctx(ctx).
			Str("room_id", timeline.RoomID).
			Str("thread_root_id", timeline.ThreadRootID).
			Int("max_rows", maxContextRows).
			Msg("Chat history read hit the row cap, older context is not visible")
	}

	return applyAnchor(ctx, timeline, history)
}

// buildContextWindow returns the history rows that make up the current context window of a
// timeline, advancing and persisting the anchor when the window has outgrown its token budget.
//
// The window grows until it exceeds the high mark, then the anchor jumps forward far enough to bring
// it under the low mark and stays put until the next overflow. Evicting one message per turn instead
// would change the prompt prefix on every request, which both unsettles the model and makes
// provider-side prompt caching impossible.
//
// A thread started from a stored turn begins with that turn. It is the oldest part of the thread's
// window, so it is the first to go when the anchor advances.
func buildContextWindow(ctx context.Context, timeline db.Timeline, seedTurnEventID string, high, low int) []db.ChatMessage {
	roomID := timeline.RoomID
	history := currentContextWindow(ctx, timeline, seedTurnEventID)
	if len(history) == 0 {
		return nil
	}

	windowTokens := estimateHistoryTokens(history)
	if windowTokens <= high {
		return history
	}

	trimmed, newAnchorID := trimToLowMark(history, low)
	if newAnchorID == 0 {
		// No usable turn boundary to move to, so leave the window alone rather than cutting the
		// conversation mid-turn. The next turn adds a boundary and this resolves itself.
		log.Warn().Ctx(ctx).
			Str("room_id", roomID).
			Str("thread_root_id", timeline.ThreadRootID).
			Int("window_tokens", windowTokens).
			Int("high_tokens", high).
			Msg("Context window is over budget but has no turn boundary to anchor to")
		return history
	}

	log.Info().Ctx(ctx).
		Str("room_id", roomID).
		Str("thread_root_id", timeline.ThreadRootID).
		Int("window_tokens", windowTokens).
		Int("trimmed_tokens", estimateHistoryTokens(trimmed)).
		Int("high_tokens", high).
		Int("low_tokens", low).
		Int("dropped_messages", len(history)-len(trimmed)).
		Int64("anchor_id", newAnchorID).
		Msg("Context window exceeded budget, advancing anchor")

	metrics.RecordChatContextAnchorAdvance()

	if err := db.SetChatContextAnchor(ctx, timeline, newAnchorID); err != nil {
		// Already logged. The window is still correct for this turn; the anchor simply is not
		// persisted, so the next turn recomputes it.
		return trimmed
	}

	return trimmed
}

// applyAnchor drops the history rows that precede the timeline's stored anchor.
//
// A missing anchor means the window starts at the oldest available row. An anchor pointing at a row
// that no longer exists — the retention cleanup deletes rows older than a week — still drops only
// the rows before it, so a quiet room degrades to a shorter window instead of breaking.
//
// An anchor past every row leaves the window empty. A reset puts a thread's anchor there, so that
// the turn the thread was started from, which the reset doesn't delete, stays out of the thread.
func applyAnchor(ctx context.Context, timeline db.Timeline, history []db.ChatMessage) []db.ChatMessage {
	anchorID, err := db.GetChatContextAnchor(ctx, timeline)
	if err != nil {
		// Already logged. The window starts at the oldest row, as without an anchor.
		return history
	}
	return fromAnchor(history, anchorID)
}

// fromAnchor returns the history rows from the anchor on, all of them if there is no anchor
func fromAnchor(history []db.ChatMessage, anchorID *int64) []db.ChatMessage {
	if anchorID == nil {
		return history
	}
	for i, msg := range history {
		if msg.ID >= *anchorID {
			return history[i:]
		}
	}
	return nil
}

// trimToLowMark finds the earliest turn boundary that brings the window under the low mark and
// returns the history from there, along with the id to anchor at.
//
// The boundary is always a user message: cutting mid-turn would strand a tool call without its
// response, which the chat API rejects. Returns a zero id when no suitable boundary exists.
func trimToLowMark(history []db.ChatMessage, low int) ([]db.ChatMessage, int64) {
	// Walk backwards accumulating tokens, remembering the earliest turn boundary that still fits
	tokens := 0
	best := -1
	for i := len(history) - 1; i >= 0; i-- {
		tokens += estimateTokens(replayText(history[i]))
		if tokens > low {
			break
		}
		if isTurnBoundary(history[i]) {
			best = i
		}
	}

	if best < 0 {
		// Nothing fits under the low mark, so fall back to the newest boundary available. This
		// keeps at least the most recent turn rather than giving up entirely.
		for i := len(history) - 1; i >= 0; i-- {
			if isTurnBoundary(history[i]) {
				best = i
				break
			}
		}
	}
	if best < 0 {
		return history, 0
	}

	return history[best:], history[best].ID
}

// isTurnBoundary reports whether a history row starts a conversation turn
func isTurnBoundary(msg db.ChatMessage) bool {
	return msg.Role == "user" && msg.MessageType == "text"
}

// recordTokenEstimateDrift compares the estimated prompt size against the count the API reported,
// so the accuracy of estimateTokens stays observable rather than assumed.
//
// The estimate must cover the whole prompt, tool definitions included, or the two sides are not
// measuring the same thing and the ratio says nothing about the estimator.
func recordTokenEstimateDrift(ctx context.Context, model string, estimated, actual int) {
	if estimated <= 0 || actual <= 0 {
		return
	}
	ratio := float64(actual) / float64(estimated)
	metrics.RecordTokenEstimateDrift(model, ratio)
	log.Debug().Ctx(ctx).
		Str("model", model).
		Int("estimated_prompt_tokens", estimated).
		Int("actual_prompt_tokens", actual).
		Float64("drift_ratio", ratio).
		Msg("Prompt token estimate compared against reported usage")
}

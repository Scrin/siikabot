package chat

import (
	"context"

	"github.com/Scrin/siikabot/aigateway"
	"github.com/Scrin/siikabot/db"
	"github.com/Scrin/siikabot/metrics"
	"github.com/rs/zerolog/log"
)

// Default context window token budget. The window grows until it exceeds the high mark, then the
// anchor jumps forward far enough to bring it under the low mark and stays put until the next
// overflow. Evicting one message per turn instead would change the prompt prefix on every request,
// which both unsettles the model and makes provider-side prompt caching impossible.
const defaultContextHighTokens = 16384
const defaultContextLowTokens = 8192

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
		total += estimateTokens(msg.Message)
	}
	return total
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

// getContextBudgetForRoom returns the high and low token marks for a room, falling back to the
// defaults when the room has no override
func getContextBudgetForRoom(ctx context.Context, roomID string) (high, low int) {
	high, low = defaultContextHighTokens, defaultContextLowTokens

	configuredHigh, configuredLow, err := db.GetRoomChatContextTokens(ctx, roomID)
	if err != nil {
		return high, low
	}
	if configuredHigh != nil && *configuredHigh > 0 {
		high = *configuredHigh
	}
	if configuredLow != nil && *configuredLow > 0 {
		low = *configuredLow
	}
	if low >= high {
		log.Warn().Ctx(ctx).
			Str("room_id", roomID).
			Int("high_tokens", high).
			Int("low_tokens", low).
			Msg("Room context low mark is not below the high mark, falling back to defaults")
		return defaultContextHighTokens, defaultContextLowTokens
	}
	return high, low
}

// currentContextWindow returns the context window as it currently stands, without advancing the
// anchor. Use this for reporting; buildContextWindow is the one that maintains the window.
func currentContextWindow(ctx context.Context, roomID string) []db.ChatMessage {
	history, err := db.GetChatHistory(ctx, roomID, maxContextRows)
	if err != nil {
		// Already logged. Continuing without history is better than refusing to answer.
		return nil
	}
	if len(history) == 0 {
		return nil
	}
	if len(history) == maxContextRows {
		log.Warn().Ctx(ctx).
			Str("room_id", roomID).
			Int("max_rows", maxContextRows).
			Msg("Chat history read hit the row cap, older context is not visible")
	}

	return applyAnchor(ctx, roomID, history)
}

// buildContextWindow returns the history rows that make up the current context window for a room,
// advancing and persisting the anchor when the window has outgrown its token budget.
func buildContextWindow(ctx context.Context, roomID string) []db.ChatMessage {
	history := currentContextWindow(ctx, roomID)
	if len(history) == 0 {
		return nil
	}

	high, low := getContextBudgetForRoom(ctx, roomID)
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
			Int("window_tokens", windowTokens).
			Int("high_tokens", high).
			Msg("Context window is over budget but has no turn boundary to anchor to")
		return history
	}

	log.Info().Ctx(ctx).
		Str("room_id", roomID).
		Int("window_tokens", windowTokens).
		Int("trimmed_tokens", estimateHistoryTokens(trimmed)).
		Int("high_tokens", high).
		Int("low_tokens", low).
		Int("dropped_messages", len(history)-len(trimmed)).
		Int64("anchor_id", newAnchorID).
		Msg("Context window exceeded budget, advancing anchor")

	metrics.RecordChatContextAnchorAdvance()

	if err := db.SetRoomChatContextAnchor(ctx, roomID, newAnchorID); err != nil {
		// Already logged. The window is still correct for this turn; the anchor simply is not
		// persisted, so the next turn recomputes it.
		return trimmed
	}

	return trimmed
}

// applyAnchor drops the history rows that precede the room's stored anchor.
//
// A missing anchor means the window starts at the oldest available row. An anchor pointing at a row
// that no longer exists — the retention cleanup deletes rows older than a week — is treated the same
// way rather than as an error, so a quiet room degrades to a shorter window instead of breaking.
func applyAnchor(ctx context.Context, roomID string, history []db.ChatMessage) []db.ChatMessage {
	anchorID, err := db.GetRoomChatContextAnchor(ctx, roomID)
	if err != nil || anchorID == nil {
		return history
	}

	for i, msg := range history {
		if msg.ID >= *anchorID {
			if i > 0 {
				return history[i:]
			}
			return history
		}
	}

	// Every row predates the anchor, which means the anchored rows have since been deleted
	log.Warn().Ctx(ctx).
		Str("room_id", roomID).
		Int64("anchor_id", *anchorID).
		Msg("Context anchor points past all available history, starting from the oldest row")
	return history
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
		tokens += estimateTokens(history[i].Message)
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
	if msg.Role != "user" {
		return false
	}
	return msg.MessageType == "text" || msg.MessageType == ""
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

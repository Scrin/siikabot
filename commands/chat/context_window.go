package chat

import (
	"context"
	"slices"
	"time"

	"github.com/Scrin/siikabot/aigateway"
	"github.com/Scrin/siikabot/db"
	"github.com/Scrin/siikabot/metrics"
	"github.com/rs/zerolog/log"
)

// messageTokenOverhead approximates the per-message framing the API adds around the content
const messageTokenOverhead = 4

// maxContextRows caps how many history rows a turn reads, for a window whose high mark is high. The
// anchor keeps the read well below it: only a timeline with no anchor yet, or one whose anchor could
// not be read or moved, reaches it.
//
// It follows the budget so that it can never decide the window in the budget's place. Every row is
// estimated at messageTokenOverhead tokens at least, so a window within the high mark has fewer rows
// than this, and a read the cap cuts short is over the high mark already: the anchor then advances as
// it would anyway. A fixed cap used to cut windows that were still within a large budget, and the
// anchor of such a window never moved again, while the cut slid forward by a turn on every turn.
func maxContextRows(high int) int {
	return high/messageTokenOverhead + 1
}

// estimateTokens approximates the token count of a string.
//
// This is deliberately a heuristic rather than a real tokeniser: an exact count would mean a
// model-specific dependency, and the counts differ between providers anyway. The budget only needs
// to be roughly right, and recordTokenEstimateDrift compares the estimate against the token count
// the API actually reports so the error stays visible.
func estimateTokens(text string) int {
	return len(text)/4 + messageTokenOverhead
}

// pixelsPerImageToken is about how many pixels of an image make a token. It is Anthropic's published
// rule, and in the range of the tile and patch schemes of other providers. A provider that honours a
// low image detail charges far less, so the estimate errs towards an image leaving the window early.
const pixelsPerImageToken = 750

// estimateImageTokens approximates the token cost of showing the model an image of the given size.
// Like estimateTokens it is a heuristic, and the drift metric says how well it fits the model.
func estimateImageTokens(width, height int) int {
	return width*height/pixelsPerImageToken + messageTokenOverhead
}

// estimateImagesTokens approximates the token cost of showing the model the given images
func estimateImagesTokens(images []db.ChatImage) int {
	total := 0
	for _, image := range images {
		total += estimateImageTokens(image.Width, image.Height)
	}
	return total
}

// estimateHistoryTokens approximates the token cost of replaying the given history rows at the time
// now
func estimateHistoryTokens(history []db.ChatMessage, now time.Time) int {
	total := 0
	for _, msg := range history {
		total += estimateRowTokens(msg, now)
	}
	return total
}

// estimateRowTokens approximates the token cost of replaying a history row at the time now: its text,
// and for a user message the images it shows, which stay in the window for as long as the message does
func estimateRowTokens(msg db.ChatMessage, now time.Time) int {
	tokens := estimateTokens(replayText(msg, now))
	if msg.Role == "user" {
		tokens += estimateImagesTokens(shownImages(msg.UserTurn()))
	}
	return tokens
}

// replayText is the text a history row is replayed as at the time now, which is also what its share
// of the token budget is measured on: a user message is replayed with its header, and a tool result
// as replayableToolResponse leaves it, cut short or replaced by its expiry marker.
//
// The budget used to measure a tool result as stored, and a window holding tool results was trimmed
// long before it reached the budget, since most of what it counted was text the model never saw.
func replayText(msg db.ChatMessage, now time.Time) string {
	switch {
	case msg.Role == "user":
		return renderUserTurn(msg.UserTurn())
	case msg.MessageType == "tool_response":
		return replayableToolResponse(msg, now)
	}
	return msg.Message
}

// historyImages returns how many images the user messages of a history show the model, and their
// estimated token cost
func historyImages(history []db.ChatMessage) (int, int) {
	count, tokens := 0, 0
	for _, msg := range history {
		if msg.Role == "user" {
			images := shownImages(msg.UserTurn())
			count += len(images)
			tokens += estimateImagesTokens(images)
		}
	}
	return count, tokens
}

// estimateMessageTokens approximates the prompt size of a built request. Image content parts are
// counted by their text only: a base64 image bears no relation to its token cost, and counting the
// data URI would swamp the estimate with a number that means nothing. The images are estimated from
// their size where they are attached, with estimateImageTokens.
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
// window. seedTurnEventID is the stored turn a thread was started from, empty for none, and high is
// the high mark of the window, which the row cap follows.
//
// The window is the history from the timeline's anchor on. A missing anchor means the window starts
// at the oldest available row. An anchor pointing at a row that no longer exists — the retention
// cleanup deletes rows older than a week — still leaves out only the rows before it, so a quiet room
// degrades to a shorter window instead of breaking.
//
// An anchor past every row leaves the window empty. A reset puts a thread's anchor there, so that
// the turn the thread was started from, which the reset doesn't delete, stays out of the thread.
func currentContextWindow(ctx context.Context, timeline db.Timeline, seedTurnEventID string, high int) []db.ChatMessage {
	// A failure is already logged, and the window then starts at the oldest row, as without an anchor
	anchorID, _ := db.GetChatContextAnchor(ctx, timeline)

	maxRows := maxContextRows(high)
	history, err := db.GetChatHistory(ctx, timeline, seedTurnEventID, anchorID, maxRows)
	if err != nil {
		// Already logged. Continuing without history is better than refusing to answer.
		return nil
	}
	if len(history) == 0 {
		return nil
	}
	if len(history) == maxRows {
		// Nothing the window needs is lost by the cut: what was read is over the high mark already,
		// so the next trim moves the anchor past the rows the read left out
		log.Debug().Ctx(ctx).
			Str("room_id", timeline.RoomID).
			Str("thread_root_id", timeline.ThreadRootID).
			Int("max_rows", maxRows).
			Msg("Chat history read hit the row cap")
	}

	return history
}

// buildContextWindow returns the history rows that make up the current context window of a
// timeline, advancing and persisting the anchor when the window has outgrown its token budget. The
// window is measured as it is replayed at the time now, the time the turn is built at.
//
// The window grows until it exceeds the high mark, then the anchor jumps forward far enough to bring
// it under the low mark and stays put until the next overflow. Evicting one message per turn instead
// would change the prompt prefix on every request, which both unsettles the model and makes
// provider-side prompt caching impossible. A tool result expiring only ever makes the window smaller,
// so the passing of time alone never moves the anchor.
//
// A thread started from a stored turn begins with that turn. It is the oldest part of the thread's
// window, so it is the first to go when the anchor advances.
func buildContextWindow(ctx context.Context, timeline db.Timeline, seedTurnEventID string, high, low int, now time.Time) []db.ChatMessage {
	roomID := timeline.RoomID
	history := currentContextWindow(ctx, timeline, seedTurnEventID, high)
	if len(history) == 0 {
		return nil
	}

	windowTokens := estimateHistoryTokens(history, now)
	if windowTokens <= high {
		return history
	}

	trimmed, newAnchorID := trimToLowMark(history, low, now)
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
		Int("trimmed_tokens", estimateHistoryTokens(trimmed, now)).
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

// loadHistoryImages reads the images the messages in a context window showed, which are shown again
// with them. The window is changed in place. An image that can't be read, or whose message has been
// redacted since, is left out, and its message says it isn't attached, as for an image that couldn't
// be downloaded when it was sent.
func loadHistoryImages(ctx context.Context, roomID string, window []db.ChatMessage) {
	if err := db.LoadChatImageData(ctx, roomID, window); err != nil {
		// Already logged. The window goes ahead without its images.
		_ = err
	}
	for i := range window {
		window[i].Images = slices.DeleteFunc(window[i].Images, func(image db.ChatImage) bool {
			return image.Data == nil
		})
	}
}

// trimToLowMark finds the earliest turn boundary that brings the window under the low mark, as it
// is replayed at the time now, and returns the history from there, along with the id to anchor at.
//
// The boundary is always a user message: cutting mid-turn would strand a tool call without its
// response, which the chat API rejects. Returns a zero id when no suitable boundary exists.
func trimToLowMark(history []db.ChatMessage, low int, now time.Time) ([]db.ChatMessage, int64) {
	// Walk backwards accumulating tokens, remembering the earliest turn boundary that still fits
	tokens := 0
	best := -1
	for i := len(history) - 1; i >= 0; i-- {
		tokens += estimateRowTokens(history[i], now)
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
// measuring the same thing and the ratio says nothing about the estimator. withImages says the
// prompt held images, whose part of the estimate comes from estimateImageTokens instead: kept apart,
// the drift of prompts without images stays comparable over time, and that of prompts with them says
// how well the image estimate fits the model.
func recordTokenEstimateDrift(ctx context.Context, model string, withImages bool, estimated, actual int) {
	if estimated <= 0 || actual <= 0 {
		return
	}
	ratio := float64(actual) / float64(estimated)
	metrics.RecordTokenEstimateDrift(model, withImages, ratio)
	log.Debug().Ctx(ctx).
		Str("model", model).
		Bool("with_images", withImages).
		Int("estimated_prompt_tokens", estimated).
		Int("actual_prompt_tokens", actual).
		Float64("drift_ratio", ratio).
		Msg("Prompt token estimate compared against reported usage")
}

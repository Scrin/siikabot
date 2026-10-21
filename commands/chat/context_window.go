package chat

import (
	"context"
	"slices"

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

// estimateHistoryTokens approximates the token cost of replaying the given history rows
func estimateHistoryTokens(history []db.ChatMessage) int {
	total := 0
	for _, msg := range history {
		total += estimateRowTokens(msg)
	}
	return total
}

// estimateRowTokens approximates the token cost of replaying a history row: its text, and for a user
// message the images it shows, which stay in the window for as long as the message does
func estimateRowTokens(msg db.ChatMessage) int {
	tokens := estimateTokens(replayText(msg))
	if msg.Role == "user" {
		tokens += estimateImagesTokens(shownImages(msg.UserTurn()))
	}
	return tokens
}

// replayText is the text a history row is replayed as, which is also what its share of the token
// budget is measured on: a user message is replayed with its header
func replayText(msg db.ChatMessage) string {
	if msg.Role == "user" {
		return renderUserTurn(msg.UserTurn())
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
		tokens += estimateRowTokens(history[i])
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

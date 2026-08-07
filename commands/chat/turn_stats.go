package chat

import (
	"context"
	"time"

	"github.com/Scrin/siikabot/aigateway"
	"github.com/Scrin/siikabot/db"
	"github.com/Scrin/siikabot/metrics"
	"github.com/rs/zerolog/log"
)

// Phase names for the turn duration breakdown
const (
	phaseModel = "model"
	phaseTools = "tools"
)

// Prompt component names for the composition breakdown
const (
	componentSystem  = "system"
	componentTools   = "tools"
	componentHistory = "history"
	componentCurrent = "current"
)

// turnStats accumulates what happened during a single chat turn.
//
// A turn can span several API calls and several rounds of tool execution, and until now the only
// record of it was a scattering of debug lines. Collecting it in one place lets a turn be summarised
// in a single log entry and its time attributed between the model and the tools.
type turnStats struct {
	iterations       int
	modelDuration    time.Duration
	toolDuration     time.Duration
	promptTokens     int
	completionTokens int
	cachedTokens     int
	toolsCalled      []string
	outcome          string
}

// recordModelCall adds the cost and duration of one API call to the turn
func (s *turnStats) recordModelCall(resp *aigateway.ChatResponse, elapsed time.Duration) {
	s.modelDuration += elapsed
	if resp == nil || resp.Usage == nil {
		return
	}
	s.promptTokens += resp.Usage.PromptTokens
	s.completionTokens += resp.Usage.CompletionTokens
	s.cachedTokens += resp.Usage.CachedPromptTokens()
}

// recordToolExecution adds a round of tool execution to the turn
func (s *turnStats) recordToolExecution(toolCalls []aigateway.ToolCall, elapsed time.Duration) {
	s.toolDuration += elapsed
	for _, call := range toolCalls {
		s.toolsCalled = append(s.toolsCalled, call.Function.Name)
	}
}

// cacheHitRate returns the share of prompt tokens served from the provider's cache
func (s *turnStats) cacheHitRate() float64 {
	if s.promptTokens == 0 {
		return 0
	}
	return float64(s.cachedTokens) / float64(s.promptTokens)
}

// finish records everything a completed turn produced: the phase metrics, the per-room usage row,
// and the single summary log entry.
//
// Unlike the metrics, neither the database nor the logs are publicly readable, so these are the
// places where a turn can be tied back to the room and user it belonged to.
func (s *turnStats) finish(ctx context.Context, roomID, sender, model string, hasImage bool, totalDuration time.Duration) {
	metrics.RecordChatTurnPhase(model, phaseModel, s.modelDuration.Seconds())
	metrics.RecordChatTurnPhase(model, phaseTools, s.toolDuration.Seconds())

	// Written on a context that outlives a cancelled turn: a turn that timed out is precisely the
	// one whose cost is worth recording
	persistCtx, cancel := persistContext(ctx)
	defer cancel()

	if err := db.SaveChatUsage(persistCtx, db.ChatUsage{
		RoomID:             roomID,
		UserID:             sender,
		Model:              model,
		PromptTokens:       s.promptTokens,
		CompletionTokens:   s.completionTokens,
		CachedPromptTokens: s.cachedTokens,
		ToolIterations:     s.iterations,
		HasImage:           hasImage,
		DurationMS:         int(totalDuration.Milliseconds()),
		Outcome:            s.outcome,
	}); err != nil {
		// Already logged. Accounting is best-effort and must never fail a turn.
		_ = err
	}

	log.Info().Ctx(ctx).
		Str("room_id", roomID).
		Str("sender", sender).
		Str("model", model).
		Bool("has_image", hasImage).
		Str("outcome", s.outcome).
		Int("tool_iterations", s.iterations).
		Strs("tools_called", s.toolsCalled).
		Int("prompt_tokens", s.promptTokens).
		Int("completion_tokens", s.completionTokens).
		Int("cached_prompt_tokens", s.cachedTokens).
		Float64("cache_hit_rate", s.cacheHitRate()).
		Float64("model_duration_sec", s.modelDuration.Seconds()).
		Float64("tool_duration_sec", s.toolDuration.Seconds()).
		Float64("total_duration_sec", totalDuration.Seconds()).
		Msg("Chat turn completed")
}

// promptComposition is the estimated token cost of each part of a built prompt.
//
// The parts are exhaustive by construction: system covers the system prompt, history the replayed
// window, current everything added for this turn (the timestamp, any reply context, the user's
// message), and tools the definitions sent alongside. They therefore sum to the whole prompt, which
// is what makes total comparable against the token count the API reports.
type promptComposition struct {
	system  int
	tools   int
	history int
	current int
}

// total is the estimated size of the entire prompt.
//
// Tool definitions have to be included: they are a field of the request rather than a message, but
// the provider counts them in prompt_tokens all the same. Leaving them out was what made the drift
// ratio read as though the estimate were three times too low.
func (c promptComposition) total() int {
	return c.system + c.tools + c.history + c.current
}

// record reports the composition, so trimming effort can be aimed at whichever part actually
// dominates rather than at whichever part is easiest to shrink
func (c promptComposition) record(ctx context.Context) {
	metrics.RecordChatPromptComponent(componentSystem, c.system)
	metrics.RecordChatPromptComponent(componentTools, c.tools)
	metrics.RecordChatPromptComponent(componentHistory, c.history)
	metrics.RecordChatPromptComponent(componentCurrent, c.current)

	log.Debug().Ctx(ctx).
		Int("system_tokens", c.system).
		Int("tools_tokens", c.tools).
		Int("history_tokens", c.history).
		Int("current_tokens", c.current).
		Msg("Prompt composition")
}

// estimateToolDefinitionTokens approximates the cost of the tool definitions sent with a request.
// They are re-sent on every call of every turn, so they are worth measuring separately.
func estimateToolDefinitionTokens(tools []aigateway.ToolDefinition) int {
	total := 0
	for _, tool := range tools {
		total += estimateTokens(tool.Function.Name)
		total += estimateTokens(tool.Function.Description)
		total += estimateTokens(string(tool.Function.Parameters))
	}
	return total
}

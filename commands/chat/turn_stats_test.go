package chat

import (
	"testing"
	"time"

	"github.com/Scrin/siikabot/aigateway"
)

func TestTurnStatsAccumulatesAcrossIterations(t *testing.T) {
	stats := &turnStats{outcome: "ok"}

	resp := func(prompt, completion, cached int) *aigateway.ChatResponse {
		return &aigateway.ChatResponse{Usage: &aigateway.Usage{
			PromptTokens:        prompt,
			CompletionTokens:    completion,
			PromptTokensDetails: &aigateway.PromptTokensDetails{CachedTokens: cached},
		}}
	}

	stats.recordModelCall(resp(1000, 50, 800), 2*time.Second)
	stats.recordModelCall(resp(1200, 30, 1100), 3*time.Second)

	if stats.promptTokens != 2200 {
		t.Errorf("expected 2200 prompt tokens, got %d", stats.promptTokens)
	}
	if stats.completionTokens != 80 {
		t.Errorf("expected 80 completion tokens, got %d", stats.completionTokens)
	}
	if stats.cachedTokens != 1900 {
		t.Errorf("expected 1900 cached tokens, got %d", stats.cachedTokens)
	}
	if stats.modelDuration != 5*time.Second {
		t.Errorf("expected 5s of model time, got %s", stats.modelDuration)
	}
}

// TestTurnStatsToleratesMissingUsage covers a failed call, where there is a duration to record but
// no response to read usage from
func TestTurnStatsToleratesMissingUsage(t *testing.T) {
	stats := &turnStats{}

	stats.recordModelCall(nil, time.Second)
	stats.recordModelCall(&aigateway.ChatResponse{}, time.Second)

	if stats.promptTokens != 0 {
		t.Errorf("expected no tokens recorded, got %d", stats.promptTokens)
	}
	if stats.modelDuration != 2*time.Second {
		t.Errorf("expected the duration to still be recorded, got %s", stats.modelDuration)
	}
}

func TestTurnStatsRecordsToolExecution(t *testing.T) {
	stats := &turnStats{}

	stats.recordToolExecution([]aigateway.ToolCall{
		{Function: aigateway.ToolFunction{Name: "get_weather"}},
		{Function: aigateway.ToolFunction{Name: "get_time"}},
	}, 4*time.Second)
	stats.recordToolExecution([]aigateway.ToolCall{
		{Function: aigateway.ToolFunction{Name: "web_search"}},
	}, time.Second)

	if stats.toolDuration != 5*time.Second {
		t.Errorf("expected 5s of tool time, got %s", stats.toolDuration)
	}
	if len(stats.toolsCalled) != 3 {
		t.Errorf("expected 3 tools recorded, got %v", stats.toolsCalled)
	}
}

func TestTurnStatsCacheHitRate(t *testing.T) {
	empty := &turnStats{}
	if got := empty.cacheHitRate(); got != 0 {
		t.Errorf("expected 0 for a turn with no prompt tokens, got %v", got)
	}

	stats := &turnStats{promptTokens: 1000, cachedTokens: 750}
	if got := stats.cacheHitRate(); got != 0.75 {
		t.Errorf("expected 0.75, got %v", got)
	}
}

// TestEstimateToolDefinitionTokens verifies the tool payload is measured, since it is re-sent on
// every call of every turn and is the reason the composition breakdown exists
func TestEstimateToolDefinitionTokens(t *testing.T) {
	if got := estimateToolDefinitionTokens(nil); got != 0 {
		t.Errorf("expected 0 for no tools, got %d", got)
	}

	tools := []aigateway.ToolDefinition{
		{Function: aigateway.FunctionSchema{
			Name:        "get_weather",
			Description: "Get the weather for a location",
			Parameters:  []byte(`{"type":"object","properties":{"location":{"type":"string"}}}`),
		}},
	}

	single := estimateToolDefinitionTokens(tools)
	if single <= 0 {
		t.Fatalf("expected a positive estimate, got %d", single)
	}
	if double := estimateToolDefinitionTokens(append(tools, tools[0])); double <= single {
		t.Errorf("expected two tools to estimate higher than one, got %d and %d", double, single)
	}
}

// TestPromptCompositionTotalIncludesTools is the regression test for a drift metric that read as
// though the estimator were three times too low.
//
// The estimate was built from the messages alone while the provider counts tool definitions in
// prompt_tokens too, so the two sides were measuring different things. The numbers below are from a
// real turn: 132 system, 469 history, 11 current, 2275 tools, against 2189 reported prompt tokens.
func TestPromptCompositionTotalIncludesTools(t *testing.T) {
	composition := promptComposition{system: 132, history: 469, current: 11, tools: 2275}

	messagesOnly := composition.system + composition.history + composition.current
	if got := composition.total(); got != messagesOnly+composition.tools {
		t.Fatalf("total() = %d, want %d", got, messagesOnly+composition.tools)
	}

	const reportedPromptTokens = 2189

	// What the metric used to compare: messages only against a full-prompt actual
	brokenRatio := float64(reportedPromptTokens) / float64(messagesOnly)
	if brokenRatio < 3 {
		t.Fatalf("expected the messages-only comparison to be wildly off, got %.2f", brokenRatio)
	}

	// What it compares now. The estimator is not perfect, but it has to be in the right region for
	// the ratio to say anything useful about the divisor.
	ratio := float64(reportedPromptTokens) / float64(composition.total())
	if ratio < 0.5 || ratio > 2 {
		t.Errorf("drift ratio %.2f is outside the range where it can inform the estimate", ratio)
	}
}

// TestPromptCompositionCoversEveryPart guards the property total() depends on: the buckets are
// exhaustive. A part of the prompt counted in none of them would silently understate the estimate.
func TestPromptCompositionCoversEveryPart(t *testing.T) {
	empty := promptComposition{}
	if got := empty.total(); got != 0 {
		t.Errorf("expected an empty composition to total 0, got %d", got)
	}

	// Each field has to move the total, or it is not being counted
	for _, tc := range []struct {
		name        string
		composition promptComposition
	}{
		{"system", promptComposition{system: 100}},
		{"tools", promptComposition{tools: 100}},
		{"history", promptComposition{history: 100}},
		{"current", promptComposition{current: 100}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.composition.total(); got != 100 {
				t.Errorf("%s is not counted in the total: got %d", tc.name, got)
			}
		})
	}
}

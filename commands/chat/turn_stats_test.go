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

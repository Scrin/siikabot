package chat

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Scrin/siikabot/aigateway"
)

func TestAsksOnlyForTools(t *testing.T) {
	toolCall := aigateway.ToolCall{ID: "call_1", Type: "function",
		Function: aigateway.ToolFunction{Name: "web_search", Arguments: `{"query":"letsencrypt"}`}}

	tests := []struct {
		name      string
		content   any
		toolCalls []aigateway.ToolCall
		want      bool
	}{
		{"an answer", "It's sunny.", nil, false},
		{"no answer and no calls", "", nil, false},
		{"a tool call", "", []aigateway.ToolCall{toolCall}, true},
		{"a tool call with no content", nil, []aigateway.ToolCall{toolCall}, true},
		{"an answer with a tool call", "It's sunny.", []aigateway.ToolCall{toolCall}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := responseWith(tt.content)
			resp.Choices[0].Message.ToolCalls = tt.toolCalls
			if got := asksOnlyForTools(resp); got != tt.want {
				t.Errorf("asksOnlyForTools() = %v, want %v", got, tt.want)
			}
		})
	}
}

// The last request of a turn out of tool iterations still offers the tools, so the provider keeps
// parsing tool calls rather than passing them on as the answer, but doesn't let the model call them,
// and tells it why
func TestToolLimitRequest(t *testing.T) {
	tools := []aigateway.ToolDefinition{{Type: "function",
		Function: aigateway.FunctionSchema{Name: "web_search", Parameters: json.RawMessage(`{"type":"object"}`)}}}
	messages := make([]aigateway.Message, 2, 8)
	messages[0] = aigateway.Message{Role: "system", Content: "You are a bot."}
	messages[1] = aigateway.Message{Role: "tool", Content: "Results", ToolCallID: "call_1"}

	cfg := seededConfig()
	req := toolLimitRequest(cfg, messages, tools, map[string]string{"iteration": "5"})

	if req.Model != cfg.Model || req.Timeout != 45*time.Second || req.Attempts != 3 {
		t.Errorf("the request is for %s with %s and %d attempts, want the turn's model and limits",
			req.Model, req.Timeout, req.Attempts)
	}
	if len(req.Tools) != 1 {
		t.Errorf("the request offers %d tools, want the turn's tools", len(req.Tools))
	}
	if req.ToolChoice != "none" {
		t.Errorf("tool_choice = %q, want none", req.ToolChoice)
	}
	if req.MaxTokens == nil || *req.MaxTokens != 2048 {
		t.Errorf("max_tokens = %v, want 2048", req.MaxTokens)
	}

	if len(req.Messages) != 3 {
		t.Fatalf("the request has %d messages, want the turn's two and the notice", len(req.Messages))
	}
	if last := req.Messages[2]; last.Role != "system" || last.Content != toolLimitNotice {
		t.Errorf("the last message is %+v, want the tool limit notice", last)
	}

	// The turn's own messages are left as they were, even with room to append in place
	if extended := messages[:3]; extended[2].Content != nil {
		t.Errorf("the turn's messages were written to: %+v", extended[2])
	}
}

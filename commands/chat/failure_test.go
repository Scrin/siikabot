package chat

import (
	"context"
	"testing"
	"time"

	"github.com/Scrin/siikabot/aigateway"
)

func responseWith(content any) *aigateway.ChatResponse {
	return &aigateway.ChatResponse{Choices: []aigateway.Choice{{
		Message:      aigateway.Message{Role: "assistant", Content: content},
		FinishReason: "stop",
	}}}
}

func TestExtractAssistantResponse(t *testing.T) {
	tests := []struct {
		name        string
		content     any
		wantAnswer  string
		wantOutcome string
	}{
		{"text", "It's sunny.", "It's sunny.", ""},
		{"a content map with text", map[string]any{"text": "It's sunny."}, "It's sunny.", ""},
		// These used to be answered with a canned text, which was then stored as something the bot
		// had said
		{"a content map without text", map[string]any{"image": "…"}, "", "empty_response"},
		{"no content", nil, "", "empty_response"},
		{"empty text", "", "", "empty_response"},
		{"only whitespace", " \n\t", "", "empty_response"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			answer, outcome := extractAssistantResponse(context.Background(), "!room:example.com",
				"@alice:example.com", "test-model", false, responseWith(tt.content))
			if answer != tt.wantAnswer || outcome != tt.wantOutcome {
				t.Errorf("extractAssistantResponse() = %q, %q, want %q, %q", answer, outcome, tt.wantAnswer, tt.wantOutcome)
			}
		})
	}
}

// Every way a turn can fail tells the room something, and a timeout says what the user can do
func TestFailureMessage(t *testing.T) {
	outcomes := []string{"config_unavailable", "request_failed", "turn_timeout", "no_choices", "tool_failed", "empty_response"}
	for _, outcome := range outcomes {
		if failureMessage(outcome) == "" {
			t.Errorf("failureMessage(%q) is empty", outcome)
		}
	}

	if got := failureMessage("turn_timeout"); got == failureMessage("request_failed") {
		t.Errorf("a timeout is reported like any other failure: %q", got)
	}
}

func TestRequestFailure(t *testing.T) {
	if got := requestFailure(context.Background()); got != "request_failed" {
		t.Errorf("requestFailure() of a live turn = %q, want request_failed", got)
	}

	expired, cancel := context.WithTimeout(context.Background(), -time.Second)
	defer cancel()
	if got := requestFailure(expired); got != "turn_timeout" {
		t.Errorf("requestFailure() of a turn out of time = %q, want turn_timeout", got)
	}

	cancelled, cancelNow := context.WithCancel(context.Background())
	cancelNow()
	if got := requestFailure(cancelled); got != "request_failed" {
		t.Errorf("requestFailure() of a cancelled turn = %q, want request_failed", got)
	}
}

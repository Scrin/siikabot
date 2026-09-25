package chat

import (
	"context"
	"testing"
	"time"

	"github.com/Scrin/siikabot/aigateway"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
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

func TestIsNoReply(t *testing.T) {
	silent := []string{"NO_REPLY", "no_reply", " NO_REPLY\n", "`NO_REPLY`", "**NO_REPLY**", "NO_REPLY."}
	for _, answer := range silent {
		if !isNoReply(answer) {
			t.Errorf("isNoReply(%q) = false, want the model staying silent", answer)
		}
	}

	answers := []string{"You're welcome!", "NO_REPLY, you're welcome", "Sure. NO_REPLY", "", "NO REPLY needed here"}
	for _, answer := range answers {
		if isNoReply(answer) {
			t.Errorf("isNoReply(%q) = true, want an answer", answer)
		}
	}
}

// Staying silent is a turn that went as it should, so its span isn't marked as failed
func TestSilentTurnIsNotAFailure(t *testing.T) {
	for outcome, wantError := range map[string]bool{"ok": false, outcomeSilent: false, "request_failed": true} {
		recorder := tracetest.NewSpanRecorder()
		provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
		_, span := provider.Tracer("test").Start(context.Background(), "chat.turn")

		(&turnStats{outcome: outcome}).recordOnSpan(span)
		span.End()

		if isError := recorder.Ended()[0].Status().Code == codes.Error; isError != wantError {
			t.Errorf("a %q turn marks its span as an error: %v, want %v", outcome, isError, wantError)
		}
	}
}

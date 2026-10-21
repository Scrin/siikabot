package chat

import (
	"context"
	"testing"

	"github.com/Scrin/siikabot/db"
	"go.opentelemetry.io/otel/trace"
)

func ctxWithSpan(t *testing.T) (context.Context, string) {
	t.Helper()

	const traceHex = "e506efc743d94756a6a31d8734aa8170"
	traceID, err := trace.TraceIDFromHex(traceHex)
	if err != nil {
		t.Fatalf("bad trace id: %v", err)
	}
	spanID, err := trace.SpanIDFromHex("991f9f4d9d2cd629")
	if err != nil {
		t.Fatalf("bad span id: %v", err)
	}

	ctx := trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	}))
	return ctx, traceHex
}

// TestReplyDebugDataCarriesTraceID is the point of T12: a reply should be self-describing, so its
// event source can be opened and the whole turn pulled up in Tempo from the id it contains.
func TestReplyDebugDataCarriesTraceID(t *testing.T) {
	ctx, traceHex := ctxWithSpan(t)

	debugData := buildDebugData(ctx, "openai/gpt-4o-mini", nil, 0)

	if debugData["trace_id"] != traceHex {
		t.Errorf("trace_id = %v, want %q", debugData["trace_id"], traceHex)
	}
	if debugData["model"] != "openai/gpt-4o-mini" {
		t.Errorf("the existing debug fields should survive, got %v", debugData)
	}
}

// TestFailureDebugDataCarriesTraceID covers the case that motivated the change most: a failed turn
// used to carry no debug data at all, so the one reply worth investigating was the only untraceable
// one.
func TestFailureDebugDataCarriesTraceID(t *testing.T) {
	ctx, traceHex := ctxWithSpan(t)

	debugData := failureDebugData(ctx, "openai/gpt-4o-mini", "turn_timeout", seededConfig())

	if debugData["trace_id"] != traceHex {
		t.Errorf("trace_id = %v, want %q", debugData["trace_id"], traceHex)
	}
	if debugData["outcome"] != "turn_timeout" {
		t.Errorf("outcome = %v, want the failure reason", debugData["outcome"])
	}
}

// TestFailureDebugDataCarriesTheConfiguration verifies a failure says what configuration its turn ran
// on, so that the limit it ran into can be read off it, and that a turn which couldn't read its
// configuration doesn't make one up
func TestFailureDebugDataCarriesTheConfiguration(t *testing.T) {
	debugData := failureDebugData(context.Background(), "openai/gpt-4o-mini", "turn_timeout", seededConfig())

	if debugData["config_id"] != int64(1) {
		t.Errorf("config_id = %v, want 1", debugData["config_id"])
	}
	config, _ := debugData["config"].(map[string]string)
	for key, want := range map[string]string{"turn_timeout": "3m", "model_timeout": "45s", "model_attempts": "3", "max_tool_iterations": "5"} {
		if config[key] != want {
			t.Errorf("config[%s] = %q, want %q", key, config[key], want)
		}
	}

	without := failureDebugData(context.Background(), "", "config_unavailable", db.ChatConfig{})
	if _, ok := without["config"]; ok {
		t.Errorf("a turn without a configuration reported one: %v", without["config"])
	}
}

// TestDebugDataWithoutASpan verifies no trace id is written when there is no span, rather than an
// all-zero value that would look like a real id and match nothing
func TestDebugDataWithoutASpan(t *testing.T) {
	debugData := buildDebugData(context.Background(), "openai/gpt-4o-mini", nil, 0)

	if _, ok := debugData["trace_id"]; ok {
		t.Errorf("wrote a trace id with no span: %v", debugData["trace_id"])
	}
}

// TestGatewayMetadataContents verifies what Cloudflare is asked to record against its own spans
func TestGatewayMetadataContents(t *testing.T) {
	metadata := gatewayMetadata("!room:example.org", "@user:example.org", 3)

	for key, want := range map[string]string{
		"room_id":   "!room:example.org",
		"user_id":   "@user:example.org",
		"iteration": "3",
	} {
		if metadata[key] != want {
			t.Errorf("%s = %q, want %q", key, metadata[key], want)
		}
	}
}

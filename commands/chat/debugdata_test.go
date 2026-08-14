package chat

import (
	"context"
	"testing"

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

	debugData := failureDebugData(ctx, "openai/gpt-4o-mini", "turn_timeout")

	if debugData["trace_id"] != traceHex {
		t.Errorf("trace_id = %v, want %q", debugData["trace_id"], traceHex)
	}
	if debugData["outcome"] != "turn_timeout" {
		t.Errorf("outcome = %v, want the failure reason", debugData["outcome"])
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

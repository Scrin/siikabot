package aigateway

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"go.opentelemetry.io/otel/trace"
)

// ctxWithSpan builds a context carrying a valid span context, without needing a tracer provider
func ctxWithSpan(t *testing.T, traceHex, spanHex string) context.Context {
	t.Helper()

	traceID, err := trace.TraceIDFromHex(traceHex)
	if err != nil {
		t.Fatalf("bad trace id: %v", err)
	}
	spanID, err := trace.SpanIDFromHex(spanHex)
	if err != nil {
		t.Fatalf("bad span id: %v", err)
	}

	return trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	}))
}

func metadataFrom(t *testing.T, req *http.Request) map[string]string {
	t.Helper()

	raw := req.Header.Get("cf-aig-metadata")
	if raw == "" {
		return nil
	}

	var decoded map[string]string
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		t.Fatalf("metadata header is not valid JSON: %v", err)
	}
	return decoded
}

// TestMetadataCarriesTraceCorrelation covers the workaround at the heart of T10.
//
// Cloudflare discards the cf-aig-otel-* headers on the REST API, so its spans cannot be nested under
// ours. Metadata does survive, so the trace and span ids ride along there instead and the two traces
// can be matched afterwards. If these ids stop being sent, the traces silently drift apart.
func TestMetadataCarriesTraceCorrelation(t *testing.T) {
	const traceHex = "bad5326e2fca4bafbead818b13dc7111"
	const spanHex = "d271e004dacacb31"

	req, _ := http.NewRequest("POST", "https://example.org", nil)
	setMetadataHeader(ctxWithSpan(t, traceHex, spanHex), req, nil)

	metadata := metadataFrom(t, req)
	if metadata["siikabot_trace_id"] != traceHex {
		t.Errorf("siikabot_trace_id = %q, want %q", metadata["siikabot_trace_id"], traceHex)
	}
	if metadata["siikabot_span_id"] != spanHex {
		t.Errorf("siikabot_span_id = %q, want %q", metadata["siikabot_span_id"], spanHex)
	}
}

// TestMetadataMergesCallerContext verifies the business context and the correlation ids coexist,
// since both travel through the single metadata header
func TestMetadataMergesCallerContext(t *testing.T) {
	req, _ := http.NewRequest("POST", "https://example.org", nil)
	setMetadataHeader(ctxWithSpan(t, "bad5326e2fca4bafbead818b13dc7111", "d271e004dacacb31"), req,
		map[string]string{"room_id": "!room:example.org", "iteration": "2"})

	metadata := metadataFrom(t, req)
	for key, want := range map[string]string{
		"room_id":           "!room:example.org",
		"iteration":         "2",
		"siikabot_trace_id": "bad5326e2fca4bafbead818b13dc7111",
	} {
		if metadata[key] != want {
			t.Errorf("%s = %q, want %q", key, metadata[key], want)
		}
	}
}

// TestMetadataOmitsIdsWithoutASpan verifies an absent span context produces no correlation ids.
//
// Without the guard the zero value would be written as an all-zero trace id, pointing Cloudflare's
// span at a trace that does not and never will exist.
func TestMetadataOmitsIdsWithoutASpan(t *testing.T) {
	req, _ := http.NewRequest("POST", "https://example.org", nil)
	setMetadataHeader(context.Background(), req, map[string]string{"room_id": "!room:example.org"})

	metadata := metadataFrom(t, req)
	if _, ok := metadata["siikabot_trace_id"]; ok {
		t.Errorf("wrote a correlation id with no span to correlate to: %v", metadata)
	}
	if metadata["room_id"] != "!room:example.org" {
		t.Error("caller metadata should still be sent when there is no span")
	}
}

// TestMetadataHeaderAbsentWhenEmpty verifies no header is sent when there is nothing to say
func TestMetadataHeaderAbsentWhenEmpty(t *testing.T) {
	req, _ := http.NewRequest("POST", "https://example.org", nil)
	setMetadataHeader(context.Background(), req, nil)

	if got := req.Header.Get("cf-aig-metadata"); got != "" {
		t.Errorf("expected no metadata header, got %q", got)
	}
}

// TestProviderFromModel verifies the provider is derived to match what Cloudflare emits, so both
// halves of a trace can be queried the same way
func TestProviderFromModel(t *testing.T) {
	for model, want := range map[string]string{
		"openai/gpt-4o-mini":       "openai",
		"google/gemini-3-flash":    "google",
		"anthropic/claude-opus-48": "anthropic",
		"@cf/meta/llama-3":         "@cf",
		"bare-model-name":          "bare-model-name",
	} {
		if got := providerFromModel(model); got != want {
			t.Errorf("providerFromModel(%q) = %q, want %q", model, got, want)
		}
	}
}

package aigateway

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Scrin/siikabot/config"
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
		// OpenRouter model ids contain a slash of their own, so the combined form has two. Only the
		// first segment is the provider, which is what Cloudflare reports on its own span.
		"openrouter/deepseek/deepseek-v4-pro": "openrouter",
		"openrouter/openai/gpt-4o-mini":       "openrouter",
		"bare-model-name":                     "bare-model-name",
	} {
		if got := providerFromModel(model); got != want {
			t.Errorf("providerFromModel(%q) = %q, want %q", model, got, want)
		}
	}
}

// TestTraceHeadersNestCloudflaresSpan covers the mechanism the Unified API was adopted for.
//
// These two headers are the only reason inference does not use the newer REST API. If they stop
// being sent, everything still works and nothing fails — Cloudflare simply generates its own trace
// id again and its spans quietly leave the waterfall, which is precisely the kind of regression
// that goes unnoticed until someone opens a trace expecting to see cost data.
func TestTraceHeadersNestCloudflaresSpan(t *testing.T) {
	const traceHex = "bad5326e2fca4bafbead818b13dc7111"
	const spanHex = "d271e004dacacb31"

	req, _ := http.NewRequest("POST", "https://example.org", nil)
	setTraceHeaders(ctxWithSpan(t, traceHex, spanHex), req)

	if got := req.Header.Get("cf-aig-otel-trace-id"); got != traceHex {
		t.Errorf("cf-aig-otel-trace-id = %q, want %q", got, traceHex)
	}
	if got := req.Header.Get("cf-aig-otel-parent-span-id"); got != spanHex {
		t.Errorf("cf-aig-otel-parent-span-id = %q, want %q", got, spanHex)
	}
}

// TestTraceHeadersOmittedWithoutASpan verifies no ids are sent when there is no span to parent to.
//
// Cloudflare validates only the format, so an all-zero id would be accepted and its span attached
// to a trace that does not exist — worse than not linking at all, because the link looks real.
func TestTraceHeadersOmittedWithoutASpan(t *testing.T) {
	req, _ := http.NewRequest("POST", "https://example.org", nil)
	setTraceHeaders(context.Background(), req)

	for _, header := range []string{"cf-aig-otel-trace-id", "cf-aig-otel-parent-span-id"} {
		if got := req.Header.Get(header); got != "" {
			t.Errorf("%s was set to %q with no span to parent to", header, got)
		}
	}
}

// TestInferenceAuthUsesTheGatewayHeader guards the split between the two endpoints' authentication.
//
// The Unified API reads the Cloudflare token from cf-aig-authorization and treats Authorization as
// the provider's own key. Putting the token in Authorization would forward it to OpenAI as if it
// were an OpenAI key, so this is not a failure that degrades gracefully.
func TestInferenceAuthUsesTheGatewayHeader(t *testing.T) {
	config.CloudflareAPIToken = "cf-token"

	req, _ := http.NewRequest("POST", "https://example.org", nil)
	setInferenceAuthHeader(req)

	if got := req.Header.Get("cf-aig-authorization"); got != "Bearer cf-token" {
		t.Errorf("cf-aig-authorization = %q", got)
	}
	if got := req.Header.Get("Authorization"); got != "" {
		t.Errorf("Authorization should be left for a provider key, got %q", got)
	}
}

// TestManagementAuthUsesTheStandardHeader verifies the log poller still authenticates the way the
// REST API on api.cloudflare.com expects, which is the opposite of the above
func TestManagementAuthUsesTheStandardHeader(t *testing.T) {
	config.CloudflareAPIToken = "cf-token"

	req, _ := http.NewRequest("GET", "https://example.org", nil)
	setManagementAuthHeader(req)

	if got := req.Header.Get("Authorization"); got != "Bearer cf-token" {
		t.Errorf("Authorization = %q", got)
	}
	if got := req.Header.Get("cf-aig-authorization"); got != "" {
		t.Errorf("cf-aig-authorization should not be sent to the REST API, got %q", got)
	}
}

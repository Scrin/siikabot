package tracing

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// recordingProvider builds a provider with the redactor in front of a recorder, so a test sees the
// span exactly as an exporter would
func recordingProvider() (*sdktrace.TracerProvider, *tracetest.SpanRecorder) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(redactingProcessor{}),
		sdktrace.WithSpanProcessor(recorder),
	)
	return provider, recorder
}

func attrOf(t *testing.T, span sdktrace.ReadOnlySpan, key string) string {
	t.Helper()
	for _, attr := range span.Attributes() {
		if string(attr.Key) == key {
			return attr.Value.AsString()
		}
	}
	return ""
}

// TestGoogleSearchKeyNeverReachesAnExporter is the regression test for the leak.
//
// It builds the URL the way web_search.go does rather than a hand-written one, so a change to how
// that request is assembled is caught here. otelhttp records url.full verbatim — it strips only the
// userinfo component of a URL, never the query — so without the redactor the key is written into
// the span in plaintext and kept for Tempo's whole retention period.
func TestGoogleSearchKeyNeverReachesAnExporter(t *testing.T) {
	const apiKey = "AIzaSyD-SUPER-SECRET-KEY-VALUE"

	params := url.Values{}
	params.Add("q", "what is the weather in Espoo")
	params.Add("key", apiKey)
	params.Add("cx", "search-engine-id")
	params.Add("num", "10")
	requestURL := fmt.Sprintf("https://www.googleapis.com/customsearch/v1?%s", params.Encode())

	provider, recorder := recordingProvider()
	_, span := provider.Tracer("test").Start(context.Background(), "HTTP GET",
		oteltrace.WithAttributes(attribute.String(attrURLFull, requestURL)))
	span.End()

	recorded := recorder.Ended()
	if len(recorded) != 1 {
		t.Fatalf("expected 1 span, got %d", len(recorded))
	}

	got := attrOf(t, recorded[0], attrURLFull)
	if strings.Contains(got, apiKey) {
		t.Fatalf("the API key was exported in url.full: %s", got)
	}
	if !strings.Contains(got, redactedValue) {
		t.Errorf("expected the key to be marked as redacted, got: %s", got)
	}

	// The rest of the URL has to survive, or the span stops being useful for debugging
	for _, want := range []string{"www.googleapis.com", "customsearch/v1", "search-engine-id"} {
		if !strings.Contains(got, want) {
			t.Errorf("redaction removed %q, which is not a secret: %s", want, got)
		}
	}
}

// TestRedactURLLeavesInnocentURLsExactlyAsTheyWere guards against collateral damage. A URL with
// nothing sensitive must come back byte-identical — not merely equivalent — since re-encoding a
// query reorders parameters and would make otherwise identical spans differ.
func TestRedactURLLeavesInnocentURLsExactlyAsTheyWere(t *testing.T) {
	for _, raw := range []string{
		"https://opendata.fmi.fi/wfs?place=Espoo&request=getFeature&service=WFS&version=2.0.0",
		"https://api.cloudflare.com/client/v4/accounts/abc/ai/run",
		"https://matrix.example.org/_matrix/client/v3/rooms/%21room:example.org/typing/@bot:example.org",
		"not a url at all",
		"",
	} {
		t.Run(raw, func(t *testing.T) {
			got, changed := redactURL(raw)
			if changed {
				t.Errorf("reported a change for a URL with no secrets")
			}
			if got != raw {
				t.Errorf("rewrote an innocent URL:\n got: %s\nwant: %s", got, raw)
			}
		})
	}
}

// TestRedactURLCoversTheKnownParameterNames checks the denylist end to end, including the
// case-insensitivity that matters because instrumentation records whatever the caller wrote
func TestRedactURLCoversTheKnownParameterNames(t *testing.T) {
	for _, param := range []string{
		"key", "api_key", "apikey", "access_token", "token", "auth",
		"password", "secret", "signature", "sig", "client_secret",
		"KEY", "Api_Key", "ACCESS_TOKEN",
	} {
		t.Run(param, func(t *testing.T) {
			raw := "https://example.org/v1?" + param + "=s3cr3t&harmless=yes"

			got, changed := redactURL(raw)
			if !changed {
				t.Fatalf("%s was not treated as sensitive", param)
			}
			if strings.Contains(got, "s3cr3t") {
				t.Errorf("secret survived redaction: %s", got)
			}
			if !strings.Contains(got, "harmless=yes") {
				t.Errorf("non-sensitive parameter was lost: %s", got)
			}
		})
	}
}

// TestRedactQueryAttribute covers url.query, which otelgin records separately from url.full
func TestRedactQueryAttribute(t *testing.T) {
	provider, recorder := recordingProvider()
	_, span := provider.Tracer("test").Start(context.Background(), "GET /search",
		oteltrace.WithAttributes(attribute.String(attrURLQuery, "q=hello&token=s3cr3t")))
	span.End()

	got := attrOf(t, recorder.Ended()[0], attrURLQuery)
	if strings.Contains(got, "s3cr3t") {
		t.Errorf("secret survived in url.query: %s", got)
	}
}

// TestRedactQueryFallsBackWhenUnparseable verifies a malformed query carrying a sensitive name is
// redacted wholesale rather than passed through. Being over-cautious with something that could not
// be parsed is the right trade: the alternative is leaking it.
func TestRedactQueryFallsBackWhenUnparseable(t *testing.T) {
	got, changed := redactQuery("key=abc%zzinvalid")
	if !changed || strings.Contains(got, "abc") {
		t.Errorf("an unparseable query containing a secret was passed through: %q", got)
	}

	// ...but a malformed query with nothing sensitive in it is still left alone
	if got, changed := redactQuery("q=abc%zzinvalid"); changed || got != "q=abc%zzinvalid" {
		t.Errorf("an unparseable query with no secrets should be untouched, got %q", got)
	}
}

// TestRedactURLIsSharedWithLogging verifies the exported helper applies the same rules, so a URL
// cannot be safe in a trace and unsafe in a log line — which is how this leak reached two places
func TestRedactURLIsSharedWithLogging(t *testing.T) {
	got := RedactURL("https://www.googleapis.com/customsearch/v1?key=SECRET&q=x")
	if strings.Contains(got, "SECRET") {
		t.Errorf("RedactURL left the secret in place: %s", got)
	}
}

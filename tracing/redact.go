package tracing

import (
	"context"
	"net/url"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Attributes that can carry a URL, and therefore a credential someone put in a query string
const (
	attrURLFull  = "url.full"
	attrURLQuery = "url.query"
)

// redactedValue replaces a secret. Deliberately not an empty string: seeing that a parameter was
// present and withheld is more useful than not seeing it at all.
const redactedValue = "REDACTED"

// sensitiveQueryParams are query parameter names whose values are credentials.
//
// Matched case-insensitively against the parameter name, because instrumentation records whatever
// the caller wrote and APIs are inconsistent about capitalisation.
var sensitiveQueryParams = map[string]bool{
	"key":           true, // Google APIs, including the Custom Search key this bot sends
	"api_key":       true,
	"apikey":        true,
	"access_token":  true,
	"auth":          true,
	"password":      true,
	"secret":        true,
	"signature":     true,
	"sig":           true,
	"token":         true,
	"x-api-key":     true,
	"client_secret": true,
}

// redactingProcessor strips credentials out of URL attributes before a span can be exported.
//
// This exists because otelhttp records url.full verbatim: it removes the userinfo component of a URL
// but leaves the query string untouched, so a key passed as a query parameter is written into the
// span as-is. Tempo is authenticated, but it is not a secret store, and a credential that reaches it
// stays there for the whole retention period.
//
// Implemented as a span processor rather than as a fix at each call site on purpose. Every
// instrumented client in the process passes through here, including ones added later by libraries
// this code does not control, so a new tool cannot reintroduce the leak by accident.
type redactingProcessor struct{}

// OnStart rewrites any URL attribute that carries a secret.
//
// Start is the right hook: the SDK applies the attributes passed to tracer.Start before invoking
// processors, and the instrumentation this guards against sets its URL attributes there. Attributes
// added later in a span's life are not covered, which is noted in redact_test.go.
func (redactingProcessor) OnStart(_ context.Context, span sdktrace.ReadWriteSpan) {
	for _, attr := range span.Attributes() {
		switch string(attr.Key) {
		case attrURLFull:
			if redacted, changed := redactURL(attr.Value.AsString()); changed {
				span.SetAttributes(attribute.String(attrURLFull, redacted))
			}
		case attrURLQuery:
			if redacted, changed := redactQuery(attr.Value.AsString()); changed {
				span.SetAttributes(attribute.String(attrURLQuery, redacted))
			}
		}
	}
}

func (redactingProcessor) OnEnd(sdktrace.ReadOnlySpan)      {}
func (redactingProcessor) Shutdown(context.Context) error   { return nil }
func (redactingProcessor) ForceFlush(context.Context) error { return nil }

// redactURL replaces the values of sensitive query parameters in a full URL, reporting whether
// anything was changed so unaffected spans are left exactly as the instrumentation wrote them
func redactURL(raw string) (string, bool) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.RawQuery == "" {
		// An unparseable URL is left alone rather than blanked: it cannot be reasoned about, and
		// discarding it would lose the one clue as to what the span was doing
		return raw, false
	}

	redacted, changed := redactQuery(parsed.RawQuery)
	if !changed {
		return raw, false
	}

	parsed.RawQuery = redacted
	return parsed.String(), true
}

// redactQuery replaces the values of sensitive parameters in an encoded query string
func redactQuery(rawQuery string) (string, bool) {
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		// Parsing is lenient and only fails on malformed escaping. Redact wholesale rather than
		// risk passing through a secret that simply failed to decode.
		if containsSensitiveName(rawQuery) {
			return redactedValue, true
		}
		return rawQuery, false
	}

	changed := false
	for name := range values {
		if sensitiveQueryParams[strings.ToLower(name)] {
			values.Set(name, redactedValue)
			changed = true
		}
	}
	if !changed {
		return rawQuery, false
	}
	return values.Encode(), true
}

// containsSensitiveName is the fallback check for a query string that would not parse
func containsSensitiveName(rawQuery string) bool {
	lowered := strings.ToLower(rawQuery)
	for name := range sensitiveQueryParams {
		if strings.Contains(lowered, name+"=") {
			return true
		}
	}
	return false
}

// RedactURL removes credentials from a URL for logging.
//
// Shares its rules with the span processor so a URL cannot be safe in a trace and unsafe in a log
// line, which is how this leak reached two places at once.
func RedactURL(raw string) string {
	redacted, _ := redactURL(raw)
	return redacted
}

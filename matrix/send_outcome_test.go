package matrix

import (
	"context"
	"testing"
	"time"

	"github.com/Scrin/siikabot/constants"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// TestSendOutcomeMarksFailuresOnTheSpan is the regression test for A4.
//
// Every failure path recorded its metric and left the span untouched, so an undelivered reply
// produced a span with an unset status — indistinguishable from success in Grafana. That made
// matrix.send the most misleading span in the system, since it sits at the end of the path a user
// complains about.
func TestSendOutcomeMarksFailuresOnTheSpan(t *testing.T) {
	failures := []constants.MatrixSendStatus{
		constants.MatrixSendFailedEncryption,
		constants.MatrixSendFailedSend,
		constants.MatrixSendFailedForbidden,
	}

	for _, status := range failures {
		t.Run(string(status), func(t *testing.T) {
			recorder := tracetest.NewSpanRecorder()
			provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))

			_, span := provider.Tracer("test").Start(context.Background(), "matrix.send")
			recordSendOutcome(span, status)
			span.End()

			ended := recorder.Ended()[0]
			if ended.Status().Code != codes.Error {
				t.Errorf("%s left the span unmarked", status)
			}
			if ended.Status().Description != string(status) {
				t.Errorf("status description = %q, want %q", ended.Status().Description, status)
			}
		})
	}
}

// TestSendOutcomeLeavesSuccessUnmarked verifies a delivered message is not reported as an error
func TestSendOutcomeLeavesSuccessUnmarked(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))

	_, span := provider.Tracer("test").Start(context.Background(), "matrix.send")
	recordSendOutcome(span, constants.MatrixSendSuccess)
	span.End()

	if code := recorder.Ended()[0].Status().Code; code == codes.Error {
		t.Errorf("a successful send was marked as an error")
	}
}

// TestAllSendStatusesAreCovered ties the test above to the constant list, so a new failure status
// cannot be added without deciding how it should appear on a span
func TestAllSendStatusesAreCovered(t *testing.T) {
	for _, status := range constants.AllMatrixSendStatuses {
		recorder := tracetest.NewSpanRecorder()
		provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))

		_, span := provider.Tracer("test").Start(context.Background(), "matrix.send")
		recordSendOutcome(span, status)
		span.End()

		isError := recorder.Ended()[0].Status().Code == codes.Error
		if want := status != constants.MatrixSendSuccess; isError != want {
			t.Errorf("%s: error status = %v, want %v", status, isError, want)
		}
	}
}

// TestDisplayNameCacheAvoidsTheHomeserver is the regression test for B1.
//
// Mention detection calls GetDisplayName for every incoming message, and each call used to be a live
// homeserver round trip on the critical path — before the bot had even decided what the message was.
// The proof here is indirect but strict: the mautrix client is nil in tests, so any call that reaches
// it panics. Returning normally means the cache was consulted and nothing was fetched.
func TestDisplayNameCacheAvoidsTheHomeserver(t *testing.T) {
	const mxid = "@siika:example.org"

	displayNameCache.Lock()
	displayNameCache.entries[mxid] = displayNameEntry{name: "SiikaBot", fetchedAt: time.Now()}
	displayNameCache.Unlock()

	t.Cleanup(func() {
		displayNameCache.Lock()
		delete(displayNameCache.entries, mxid)
		displayNameCache.Unlock()
	})

	// Called repeatedly because the real cost was per message, not per process
	for range 100 {
		if got := GetDisplayName(context.Background(), mxid); got != "SiikaBot" {
			t.Fatalf("GetDisplayName = %q, want %q", got, "SiikaBot")
		}
	}
}

// TestDisplayNameCacheExpires verifies a stale entry stops being served, so a renamed user is
// picked up without a restart rather than being pinned for the life of the process
func TestDisplayNameCacheExpires(t *testing.T) {
	stale := displayNameEntry{name: "Old Name", fetchedAt: time.Now().Add(-2 * displayNameTTL)}
	if stale.fresh() {
		t.Error("an entry older than the TTL was treated as fresh")
	}

	current := displayNameEntry{name: "New Name", fetchedAt: time.Now()}
	if !current.fresh() {
		t.Error("a just-fetched entry was treated as stale")
	}
}

// TestDisplayNameLookupRoundTrips covers the cache store and read under its lock
func TestDisplayNameLookupRoundTrips(t *testing.T) {
	const mxid = "@roundtrip:example.org"

	if _, ok := lookupDisplayName(mxid); ok {
		t.Fatal("found an entry that was never stored")
	}

	displayNameCache.Lock()
	displayNameCache.entries[mxid] = displayNameEntry{name: "Stored", fetchedAt: time.Now()}
	displayNameCache.Unlock()
	t.Cleanup(func() {
		displayNameCache.Lock()
		delete(displayNameCache.entries, mxid)
		displayNameCache.Unlock()
	})

	entry, ok := lookupDisplayName(mxid)
	if !ok || entry.name != "Stored" {
		t.Errorf("lookupDisplayName = %+v, %v", entry, ok)
	}
}

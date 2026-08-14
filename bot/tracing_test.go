package bot

import (
	"context"
	"os"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// testExporter collects spans for the whole package.
//
// It has to be installed once rather than per test: otel.Tracer returns a tracer that delegates to
// the global provider, and it binds to the first real provider installed. Swapping providers per
// test leaves the package's tracer pointing at the first one, so every later test silently records
// nothing.
var testExporter = tracetest.NewInMemoryExporter()

func TestMain(m *testing.M) {
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSyncer(testExporter)))
	os.Exit(m.Run())
}

// recordSpans clears previously collected spans and returns the exporter
func recordSpans(t *testing.T) *tracetest.InMemoryExporter {
	t.Helper()
	testExporter.Reset()
	return testExporter
}

// waitForSpans polls until the expected number of spans has been exported, since traced runs its
// work on a goroutine
func waitForSpans(t *testing.T, exporter *tracetest.InMemoryExporter, count int) []sdktrace.ReadOnlySpan {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if spans := exporter.GetSpans().Snapshots(); len(spans) >= count {
			return spans
		}
		time.Sleep(5 * time.Millisecond)
	}

	t.Fatalf("expected %d spans, got %d", count, len(exporter.GetSpans()))
	return nil
}

// TestTracedSpanCoversTheWholeGoroutine is the regression test for the mistake this helper exists to
// prevent.
//
// Commands are dispatched with `go`, so a span started in the dispatcher and ended with a defer there
// closes the instant the dispatcher returns — every span records a fraction of a millisecond while the
// real work runs on untraced. The failure is quiet: the traces look complete and the durations look
// fast, which is worse than no traces at all.
func TestTracedSpanCoversTheWholeGoroutine(t *testing.T) {
	exporter := recordSpans(t)

	const work = 100 * time.Millisecond
	done := make(chan struct{})

	traced(context.Background(), "command.test", nil, func(ctx context.Context) {
		time.Sleep(work)
		close(done)
	})

	<-done
	spans := waitForSpans(t, exporter, 1)

	span := spans[0]
	if span.Name() != "command.test" {
		t.Errorf("expected span name %q, got %q", "command.test", span.Name())
	}

	duration := span.EndTime().Sub(span.StartTime())
	if duration < work {
		t.Errorf("span covered only %s of %s of work — it is being ended in the dispatcher, "+
			"not inside the goroutine", duration, work)
	}
}

// TestTracedSpanIsARoot verifies these spans start their own trace. Matrix gives no inbound context,
// so anything else would mean silently attaching to whatever happened to be on the context.
func TestTracedSpanIsARoot(t *testing.T) {
	exporter := recordSpans(t)

	done := make(chan struct{})
	traced(context.Background(), "command.test", nil, func(ctx context.Context) { close(done) })
	<-done

	span := waitForSpans(t, exporter, 1)[0]

	if span.Parent().IsValid() {
		t.Errorf("expected a root span, got one parented to %s", span.Parent().SpanID())
	}
	if !span.SpanContext().IsValid() {
		t.Error("expected the span to be recording with a valid context")
	}
}

// TestTracedPassesSpanContextToTheWork verifies the work receives a context carrying the span, which
// is what lets everything underneath nest correctly
func TestTracedPassesSpanContextToTheWork(t *testing.T) {
	recordSpans(t)

	var seen trace.SpanContext
	done := make(chan struct{})

	traced(context.Background(), "command.test", nil, func(ctx context.Context) {
		seen = trace.SpanContextFromContext(ctx)
		close(done)
	})
	<-done

	if !seen.IsValid() {
		t.Error("the work was given a context with no span in it, so nothing beneath it can nest")
	}
}

// TestTracedRecordsMessageAttributes verifies the room and sender reach the span, since a trace that
// cannot say which room it belonged to is markedly less useful to open
func TestTracedRecordsMessageAttributes(t *testing.T) {
	exporter := recordSpans(t)

	attrs := messageAttrs("!room:example.org", "@user:example.org", "$event")
	done := make(chan struct{})

	traced(context.Background(), "command.test", attrs, func(ctx context.Context) { close(done) })
	<-done

	got := map[attribute.Key]string{}
	for _, attr := range waitForSpans(t, exporter, 1)[0].Attributes() {
		got[attr.Key] = attr.Value.AsString()
	}

	for key, want := range map[attribute.Key]string{
		"matrix.room_id":  "!room:example.org",
		"matrix.sender":   "@user:example.org",
		"matrix.event_id": "$event",
	} {
		if got[key] != want {
			t.Errorf("attribute %s = %q, want %q", key, got[key], want)
		}
	}
}

// TestTracedConcurrentDispatchesGetSeparateTraces verifies two messages handled at once do not end up
// sharing a trace
func TestTracedConcurrentDispatchesGetSeparateTraces(t *testing.T) {
	exporter := recordSpans(t)

	done := make(chan struct{}, 2)
	for range 2 {
		traced(context.Background(), "command.test", nil, func(ctx context.Context) { done <- struct{}{} })
	}
	<-done
	<-done

	spans := waitForSpans(t, exporter, 2)
	if spans[0].SpanContext().TraceID() == spans[1].SpanContext().TraceID() {
		t.Error("concurrent dispatches shared a trace id; each message should start its own trace")
	}
}

package tracing

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func recorderOnly() (*sdktrace.TracerProvider, *tracetest.SpanRecorder) {
	recorder := tracetest.NewSpanRecorder()
	return sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder)), recorder
}

// TestRunRecordsAPanicAndKeepsGoing covers the behaviour change in B3.
//
// A panic in a handler goroutine used to take the whole process down, which meant the trace
// explaining it was still sitting in the batch processor's queue and died with it — the one trace
// you would actually want was guaranteed to be missing. This asserts both halves: the panic does not
// escape, and the span says what happened.
func TestRunRecordsAPanicAndKeepsGoing(t *testing.T) {
	provider, recorder := recorderOnly()

	Run(context.Background(), provider.Tracer("test"), "command.explodes", func(context.Context) {
		panic("boom")
	})

	ended := recorder.Ended()
	if len(ended) != 1 {
		t.Fatalf("expected the span to be ended exactly once, got %d", len(ended))
	}

	span := ended[0]
	if span.Status().Code != codes.Error {
		t.Errorf("expected an error status, got %v", span.Status().Code)
	}
	if len(span.Events()) == 0 {
		t.Fatal("expected the panic to be recorded as an event on the span")
	}
	if name := span.Events()[0].Name; name != "exception" {
		t.Errorf("expected an exception event, got %q", name)
	}
}

// TestRunLeavesASuccessfulSpanUnmarked verifies the recovery path does not colour ordinary work
func TestRunLeavesASuccessfulSpanUnmarked(t *testing.T) {
	provider, recorder := recorderOnly()

	ran := false
	Run(context.Background(), provider.Tracer("test"), "command.fine", func(context.Context) {
		ran = true
	})

	if !ran {
		t.Fatal("fn was not called")
	}
	if code := recorder.Ended()[0].Status().Code; code == codes.Error {
		t.Errorf("a successful span was marked as an error")
	}
}

// TestRunGivesFnTheSpanContext verifies work inside fn is attributed to the span rather than
// escaping into a trace of its own, which is the whole reason the helper exists
func TestRunGivesFnTheSpanContext(t *testing.T) {
	provider, recorder := recorderOnly()

	var innerTraceID string
	Run(context.Background(), provider.Tracer("test"), "outer", func(ctx context.Context) {
		innerTraceID = TraceID(ctx)
	})

	if innerTraceID == "" {
		t.Fatal("fn received a context with no span")
	}
	if want := recorder.Ended()[0].SpanContext().TraceID().String(); innerTraceID != want {
		t.Errorf("fn ran under trace %s, span was in %s", innerTraceID, want)
	}
}

// TestRecoverReturnsNilWithoutAPanic verifies the helper is safe to defer unconditionally
func TestRecoverReturnsNilWithoutAPanic(t *testing.T) {
	provider, _ := recorderOnly()
	_, span := provider.Tracer("test").Start(context.Background(), "quiet")
	defer span.End()

	if got := Recover(context.Background(), span); got != nil {
		t.Errorf("expected nil with no panic in flight, got %v", got)
	}
}

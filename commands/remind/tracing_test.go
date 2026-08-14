package remind

import (
	"context"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// TestDeliveryContextDetachesTheSchedulingSpan is the regression test for A2.
//
// A reminder can fire days after the command that scheduled it. Reusing the captured context makes
// the delivery a child of a span that ended days earlier, which stretches that trace across the
// whole interval and files the delivery under the wrong operation.
func TestDeliveryContextDetachesTheSchedulingSpan(t *testing.T) {
	provider := sdktrace.NewTracerProvider()
	schedulingCtx, span := provider.Tracer("test").Start(context.Background(), "command.remind")
	defer span.End()

	scheduling := span.SpanContext()
	if !scheduling.IsValid() {
		t.Fatal("test setup produced no span context")
	}

	deliverCtx, link := deliveryContext(schedulingCtx)

	if sc := trace.SpanContextFromContext(deliverCtx); sc.IsValid() {
		t.Errorf("delivery context still carries a span: %s", sc.SpanID())
	}

	// The causal relationship is kept, as a link — which is what "caused by, but not part of" means
	if link.SpanContext.SpanID() != scheduling.SpanID() {
		t.Errorf("link points at %s, want the scheduling span %s",
			link.SpanContext.SpanID(), scheduling.SpanID())
	}
	if link.SpanContext.TraceID() != scheduling.TraceID() {
		t.Errorf("link is in the wrong trace")
	}
}

// TestDeliveryContextHandlesNoSpan covers reminders restored at startup, whose context never had a
// span. This is the path that made the same operation trace two different ways.
func TestDeliveryContextHandlesNoSpan(t *testing.T) {
	deliverCtx, link := deliveryContext(context.Background())

	if trace.SpanContextFromContext(deliverCtx).IsValid() {
		t.Error("expected no span context")
	}
	if link.SpanContext.IsValid() {
		t.Error("expected no link when there was nothing to link to")
	}
}

// TestDeliveryContextKeepsContextValues verifies detaching the span does not discard the logging
// fields threaded through the context, which every log line on the delivery path depends on
func TestDeliveryContextKeepsContextValues(t *testing.T) {
	type key struct{}
	ctx := context.WithValue(context.Background(), key{}, "kept")

	deliverCtx, _ := deliveryContext(ctx)

	if got := deliverCtx.Value(key{}); got != "kept" {
		t.Errorf("context values were lost, got %v", got)
	}
}

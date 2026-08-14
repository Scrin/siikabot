package bot

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/Scrin/siikabot/tracing"
)

var tracer = tracing.Tracer("github.com/Scrin/siikabot/bot")

// traced runs fn in its own goroutine under a new root span, named for the work being done.
//
// The span is started and ended *inside* the goroutine, which is the whole point of this helper.
// Starting it in the dispatcher and deferring the end there would close every span the moment the
// dispatcher returned, so each one would record a fraction of a millisecond while the real work
// carried on untraced — plausible-looking data that is entirely wrong.
//
// Matrix gives us no inbound trace context, so these are genuine roots rather than children.
func traced(ctx context.Context, name string, attrs []attribute.KeyValue, fn func(context.Context)) {
	go func() {
		ctx, span := tracer.Start(ctx, name,
			trace.WithSpanKind(trace.SpanKindConsumer),
			trace.WithAttributes(attrs...),
		)
		defer span.End()

		fn(ctx)
	}()
}

// messageAttrs describes the Matrix message that triggered a span.
//
// Room and sender are attached deliberately: Tempo is authenticated, and a trace with no idea which
// room or user it belonged to is far less useful to open.
func messageAttrs(roomID, sender, eventID string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("matrix.room_id", roomID),
		attribute.String("matrix.sender", sender),
		attribute.String("matrix.event_id", eventID),
	}
}

// tracedBackground runs a single iteration of a background job under its own root span.
//
// Background work has no triggering message to inherit from, and attaching it to the long-lived bot
// context would produce a span that never ends.
func tracedBackground(ctx context.Context, name string, fn func(context.Context)) {
	ctx, span := tracer.Start(ctx, name, trace.WithSpanKind(trace.SpanKindInternal))
	defer span.End()

	fn(ctx)
}

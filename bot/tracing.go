package bot

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/Scrin/siikabot/tracing"
)

var tracer = tracing.Tracer("github.com/Scrin/siikabot/bot")

// traced runs fn in its own goroutine under a span named for the work being done.
//
// The span is started and ended inside the goroutine — see tracing.Run, which also recovers a panic
// in fn so a bad handler records what happened instead of taking the process down with it.
func traced(ctx context.Context, name string, attrs []attribute.KeyValue, fn func(context.Context)) {
	go tracing.Run(ctx, tracer, name, fn,
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(attrs...),
	)
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

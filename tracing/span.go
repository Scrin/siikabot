package tracing

import (
	"context"
	"fmt"
	"runtime/debug"

	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Run executes fn inside a new span, ending it afterwards and surviving a panic in fn.
//
// The span is started inside this call rather than by the caller so that work handed to a goroutine
// is covered for its whole life. Starting a span at the dispatch site and ending it there closes it
// the moment dispatch returns, leaving every span recording a fraction of a millisecond while the
// real work carries on untraced — data that looks plausible and is entirely wrong.
func Run(ctx context.Context, tracer trace.Tracer, name string, fn func(context.Context), opts ...trace.SpanStartOption) {
	ctx, span := tracer.Start(ctx, name, opts...)
	// Ends last: the recover below has to record on the span before it closes
	defer span.End()
	defer Recover(ctx, span)

	fn(ctx)
}

// Recover records a panic on a span and stops it unwinding any further. Call it from a deferred
// function; it returns the recovered value, or nil when there was no panic.
//
// Returning the value rather than swallowing it lets a caller do its own cleanup — a tool call, for
// instance, still has to produce a response for the model even when its handler panicked.
//
// Recovering at all is a deliberate change of behaviour. A panic in a handler goroutine used to take
// the whole process down, which meant the trace explaining it was still sitting in the batch
// processor's queue and died with it. A bot that stays up and records what happened is worth more
// than one that exits without saying why.
func Recover(ctx context.Context, span trace.Span) any {
	recovered := recover()
	if recovered == nil {
		return nil
	}

	stack := debug.Stack()
	err := fmt.Errorf("panic: %v", recovered)

	span.RecordError(err, trace.WithStackTrace(true))
	span.SetStatus(codes.Error, "panic")

	log.Error().Ctx(ctx).
		Interface("panic", recovered).
		Str("stack", string(stack)).
		Msg("Recovered from a panic")

	return recovered
}

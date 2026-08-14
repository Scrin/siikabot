package remind

import (
	"context"

	"github.com/Scrin/siikabot/tracing"
	"go.opentelemetry.io/otel/trace"
)

var tracer = tracing.Tracer("github.com/Scrin/siikabot/commands/remind")

// deliveryContext prepares the context a reminder will be delivered under, separating it from the
// span that scheduled it.
//
// A reminder can fire days after the command that created it, and the context captured at scheduling
// time still carries that command's span. Used as-is, the delivery is recorded as a child of a span
// that ended days earlier, stretching that trace across the whole interval — and reminders restored
// at startup carry no span at all, so the same operation was traced two different ways depending on
// whether a restart happened in between.
//
// The returned link preserves what is genuinely true: the delivery was caused by that command,
// without being part of it. Everything else on the context — logging fields, cancellation — is kept.
func deliveryContext(ctx context.Context) (context.Context, trace.Link) {
	return trace.ContextWithSpanContext(ctx, trace.SpanContext{}), trace.LinkFromContext(ctx)
}

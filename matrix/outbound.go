package matrix

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/Scrin/siikabot/constants"
	"github.com/Scrin/siikabot/metrics"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/crypto"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// Messages are sent one at a time from a single queue for every room, so a message that can't be
// sent holds up all the others behind it. A failed attempt is retried only when another one could
// go differently, with a growing wait between attempts, and a message is given up once its deadline
// passes.

// defaultSendTimeout is how long a queued message may take to go out, counted from when it was
// queued: long enough to ride out a restart of the homeserver, short enough that an outage doesn't
// hold up the queue for good
const defaultSendTimeout = 2 * time.Minute

// A failed attempt is retried after firstRetryDelay, and each further retry waits twice as long as
// the one before it, up to maxRetryDelay
const (
	firstRetryDelay = 500 * time.Millisecond
	maxRetryDelay   = 15 * time.Second
)

type outboundEvent struct {
	ctx       context.Context
	RoomID    string
	EventType string
	Content   simpleMessage
	done      chan<- string
	// target, when set, places the message and may cancel it, both just before it is sent
	target *Target
	// deadline is when the message is given up if it hasn't gone out yet
	deadline time.Time
}

// newOutboundEvent builds a message for the queue, with the deadline it has to go out by
func newOutboundEvent(ctx context.Context, roomID string, message simpleMessage, target *Target, done chan<- string) outboundEvent {
	timeout := defaultSendTimeout
	if target != nil && target.Timeout > 0 {
		timeout = target.Timeout
	}
	return outboundEvent{
		ctx:       ctx,
		RoomID:    roomID,
		EventType: "m.room.message",
		Content:   message,
		done:      done,
		target:    target,
		deadline:  time.Now().Add(timeout),
	}
}

// content returns the message to send, placed at its target if it has one
func (e outboundEvent) content() simpleMessage {
	msg := e.Content
	if e.target != nil {
		msg.RelatesTo = e.target.relatesTo(e.RoomID)
	}
	return msg
}

func processOutboundEvents(ctx context.Context) {
	for evt := range outboundEvents {
		sendOutboundEvent(ctx, evt)
	}
}

// recordSendOutcome records how a send ended, on the metric and on the span alike.
//
// The failure paths all recorded the metric and left the span untouched, so a reply that was never
// delivered produced a span with an unset status — rendered exactly like a success. That made
// matrix.send the most misleading span in the system, sitting as it does at the end of the path a
// user complains about.
//
// A message dropped because it was no longer wanted isn't a failure: nobody wanted it sent.
func recordSendOutcome(span trace.Span, status constants.MatrixSendStatus) {
	metrics.RecordMatrixMessageSent(status)
	if status != constants.MatrixSendSuccess && status != constants.MatrixSendDropped {
		span.SetStatus(codes.Error, string(status))
	}
}

// sendOutboundEvent delivers a single queued event to the homeserver, and reports how that ended to
// whoever queued it: the event ID it was sent as, or an empty one if it wasn't sent.
//
// Split out from the loop so the span can be ended with a defer: the body has several early exits,
// and inside a loop those were labelled continues that no defer would cover.
func sendOutboundEvent(ctx context.Context, evt outboundEvent) {
	// Continue the caller's trace rather than starting a fresh one. Sends are queued and delivered
	// on this goroutine, so without carrying the context across the channel a reply would be traced
	// separately from the work that produced it.
	//
	// The caller's cancellation is deliberately dropped: the turn that queued a message is often
	// finished by the time it goes out, and a cancelled turn must not stop its reply being delivered.
	// The message's own deadline takes its place, so that nothing goes out after it, however far a
	// send has got by then.
	ctx, span := tracer.Start(context.WithoutCancel(evt.ctx), "matrix.send",
		trace.WithSpanKind(trace.SpanKindProducer),
		trace.WithAttributes(
			attribute.String("matrix.room_id", evt.RoomID),
			attribute.String("matrix.event_type", evt.EventType),
		))
	defer span.End()
	ctx, cancel := context.WithDeadline(ctx, evt.deadline)
	defer cancel()

	// Whichever way the send ends, whoever waits for it hears exactly once
	var eventID string
	defer func() {
		if evt.done != nil {
			evt.done <- eventID
		}
	}()

	// Decided now rather than when the event was queued, since the queue can hold it for a while
	if evt.target.cancelled() {
		log.Debug().Ctx(ctx).Str("room_id", evt.RoomID).Str("event_type", evt.EventType).Msg("Dropping an outbound event that is no longer wanted")
		recordSendOutcome(span, constants.MatrixSendDropped)
		return
	}
	if !time.Now().Before(evt.deadline) {
		log.Warn().Ctx(ctx).Str("room_id", evt.RoomID).Str("event_type", evt.EventType).Msg("Giving up an outbound event that waited past its deadline")
		recordSendOutcome(span, constants.MatrixSendTimedOut)
		return
	}

	startTime := time.Now()
	metrics.SetMatrixOutboundQueueDepth(len(outboundEvents))

	// Placed now too, for the same reason: whether it goes out as a reply depends on what has been
	// posted since the message it answers, which can change while it waits
	message := evt.content()
	if evt.target != nil {
		asReply := message.RelatesTo.GetNonFallbackReplyTo() != ""
		span.SetAttributes(attribute.Bool("siikabot.matrix.as_reply", asReply))
		log.Debug().Ctx(ctx).
			Str("room_id", evt.RoomID).
			Str("thread_root_id", evt.target.ThreadRootID).
			Str("in_reply_to", evt.target.InReplyTo).
			Bool("as_reply", asReply).
			Msg("Placing an outbound message")
	}

	roomID := id.RoomID(evt.RoomID)
	var evtType event.Type
	var evtContent any
	status := withRetries(ctx, evt, func(ctx context.Context) (constants.MatrixSendStatus, bool, time.Duration) {
		var err error
		evtType, evtContent, err = encryptForRoom(ctx, roomID, event.NewEventType(evt.EventType), message)
		if err != nil {
			log.Warn().Ctx(ctx).Err(err).Str("room_id", evt.RoomID).Msg("Failed to encrypt message")
			return constants.MatrixSendFailedEncryption, true, 0
		}
		return constants.MatrixSendSuccess, false, 0
	})

	if status == constants.MatrixSendSuccess {
		// Every attempt uses the same transaction ID, so that when an attempt got through but its
		// response was lost, the homeserver takes the next one as the same message, not a new one
		txnID := client.TxnID()
		status = withRetries(ctx, evt, func(ctx context.Context) (constants.MatrixSendStatus, bool, time.Duration) {
			resp, err := client.SendMessageEvent(ctx, roomID, evtType, evtContent, mautrix.ReqSendEvent{TransactionID: txnID})
			if err != nil {
				status, retry, wait := sendFailure(err)
				// A failure that is retried is a warning; giving up is logged as an error below
				logEvent := log.Error()
				if retry {
					logEvent = log.Warn()
				}
				logEvent.Ctx(ctx).Err(err).
					Str("room_id", evt.RoomID).
					Str("status", string(status)).
					Bool("retry", retry).
					Msg("Failed to send message")
				if wait > 0 {
					metrics.RecordMatrixRateLimitRetry()
				}
				return status, retry, wait
			}
			// Recorded before anyone learns the message is out, so that the next message placed
			// sees it even before its copy comes back through sync
			recordLatest(evt.target.timeline(evt.RoomID), resp.EventID.String())
			eventID = resp.EventID.String()
			return constants.MatrixSendSuccess, false, 0
		})
	}

	recordSendOutcome(span, status)
	switch status {
	case constants.MatrixSendSuccess:
		metrics.RecordMatrixMessageLatency(time.Since(startTime).Seconds())
	case constants.MatrixSendDropped:
		log.Debug().Ctx(ctx).Str("room_id", evt.RoomID).Msg("Dropped an outbound event that stopped being wanted while it was retried")
	default:
		log.Error().Ctx(ctx).
			Str("room_id", evt.RoomID).
			Str("status", string(status)).
			Dur("elapsed", time.Since(startTime)).
			Msg("Gave up sending an outbound event")
	}
}

// encryptForRoom returns an event as it is sent to a room: encrypted for the room's members in an
// encrypted room, and as it is in any other
func encryptForRoom(ctx context.Context, roomID id.RoomID, evtType event.Type, content any) (event.Type, any, error) {
	isEncrypted, err := stateStore.IsEncrypted(ctx, roomID)
	if err != nil {
		return event.Type{}, nil, fmt.Errorf("failed to check if room is encrypted: %w", err)
	}
	if !isEncrypted {
		return evtType, content, nil
	}

	encrypted, err := olmMachine.EncryptMegolmEvent(ctx, roomID, evtType, content)
	// These three errors mean we have to make a new Megolm session
	if errors.Is(err, crypto.SessionExpired) || errors.Is(err, crypto.SessionNotShared) || errors.Is(err, crypto.NoGroupSession) {
		members, err := stateStore.GetRoomMembers(ctx, roomID)
		if err != nil {
			return event.Type{}, nil, fmt.Errorf("failed to get room members: %w", err)
		}
		if err := olmMachine.ShareGroupSession(ctx, roomID, members); err != nil {
			return event.Type{}, nil, fmt.Errorf("failed to share group session: %w", err)
		}
		encrypted, err = olmMachine.EncryptMegolmEvent(ctx, roomID, evtType, content)
	}
	if err != nil {
		return event.Type{}, nil, fmt.Errorf("failed to encrypt: %w", err)
	}
	return event.EventEncrypted, encrypted, nil
}

// withRetries makes attempts at a step of sending an event until one succeeds, one fails in a way
// another attempt wouldn't change, or the next attempt would come after the event's deadline. The
// waits between attempts grow (see retryDelay), unless a failure says how long to wait. An event
// that is no longer wanted when its next attempt comes up is dropped.
//
// Returns the status the last attempt ended with, MatrixSendTimedOut if the time ran out first, or
// MatrixSendDropped.
func withRetries(ctx context.Context, evt outboundEvent, attempt func(context.Context) (status constants.MatrixSendStatus, retry bool, wait time.Duration)) constants.MatrixSendStatus {
	for attempts := 1; ; attempts++ {
		status, retry, wait := attempt(ctx)
		if status == constants.MatrixSendSuccess || !retry {
			return status
		}
		if wait <= 0 {
			wait = retryDelay(attempts)
		}
		if time.Now().Add(wait).After(evt.deadline) {
			return constants.MatrixSendTimedOut
		}

		timer := time.NewTimer(wait)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return constants.MatrixSendTimedOut
		}
		if evt.target.cancelled() {
			return constants.MatrixSendDropped
		}
	}
}

// retryDelay is how long to wait after the given attempt, counted from 1, before the next one
func retryDelay(attempt int) time.Duration {
	delay := firstRetryDelay
	for i := 1; i < attempt && delay < maxRetryDelay; i++ {
		delay *= 2
	}
	return min(delay, maxRetryDelay)
}

// sendFailure reads why an attempt to send failed: the status it counts as, whether another attempt
// could go differently, and how long the homeserver asks to wait before one, if it says.
//
// Only failures on the homeserver's side, and requests that got no answer at all, are worth another
// attempt. Anything else is the request's own fault, and it would fail the same way again.
func sendFailure(err error) (constants.MatrixSendStatus, bool, time.Duration) {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		// The deadline cut the attempt short
		return constants.MatrixSendTimedOut, false, 0
	}

	var httpErr mautrix.HTTPError
	if !errors.As(err, &httpErr) {
		return constants.MatrixSendFailedSend, false, 0
	}
	if httpErr.Response == nil {
		// No answer at all: the homeserver, or the way to it, is down
		return constants.MatrixSendFailedSend, true, 0
	}

	var errCode string
	if httpErr.RespError != nil {
		errCode = httpErr.RespError.ErrCode
	}
	switch {
	case errCode == "M_FORBIDDEN":
		return constants.MatrixSendFailedForbidden, false, 0
	case errCode == "M_LIMIT_EXCEEDED" || httpErr.IsStatus(http.StatusTooManyRequests):
		return constants.MatrixSendFailedSend, true, retryAfter(httpErr)
	case httpErr.Response.StatusCode >= http.StatusInternalServerError:
		return constants.MatrixSendFailedSend, true, 0
	default:
		return constants.MatrixSendFailedSend, false, 0
	}
}

// retryAfter is how long the homeserver asks a rate limited request to wait before trying again, or
// 0 if it doesn't say
func retryAfter(httpErr mautrix.HTTPError) time.Duration {
	if httpErr.RespError != nil {
		if ms, ok := httpErr.RespError.ExtraData["retry_after_ms"].(float64); ok && ms > 0 {
			return time.Duration(ms) * time.Millisecond
		}
	}
	if seconds, err := strconv.Atoi(httpErr.Response.Header.Get("Retry-After")); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	return 0
}

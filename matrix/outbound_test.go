package matrix

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/Scrin/siikabot/constants"
	"maunium.net/go/mautrix"
)

func TestRetryDelayBacksOff(t *testing.T) {
	want := []time.Duration{
		500 * time.Millisecond, time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second,
		15 * time.Second, 15 * time.Second,
	}
	for i, delay := range want {
		if got := retryDelay(i + 1); got != delay {
			t.Errorf("retryDelay(%d) = %v, want %v", i+1, got, delay)
		}
	}
	if got := retryDelay(1000); got != maxRetryDelay {
		t.Errorf("retryDelay(1000) = %v, want it capped at %v", got, maxRetryDelay)
	}
}

// httpFailure is a request that got the given answer
func httpFailure(status int, errCode string, extra map[string]any, header http.Header) error {
	err := mautrix.HTTPError{Response: &http.Response{StatusCode: status, Header: header}}
	if errCode != "" {
		err.RespError = &mautrix.RespError{ErrCode: errCode, StatusCode: status, ExtraData: extra}
	}
	return err
}

func TestSendFailure(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus constants.MatrixSendStatus
		wantRetry  bool
		wantWait   time.Duration
	}{
		{
			name:       "no answer from the homeserver",
			err:        mautrix.HTTPError{Message: "request error", WrappedError: &url.Error{Op: "Post", URL: "https://example.com", Err: errors.New("connection refused")}},
			wantStatus: constants.MatrixSendFailedSend, wantRetry: true,
		},
		{
			name:       "the homeserver failed",
			err:        httpFailure(http.StatusBadGateway, "", nil, nil),
			wantStatus: constants.MatrixSendFailedSend, wantRetry: true,
		},
		{
			name:       "rate limited, saying for how long",
			err:        httpFailure(http.StatusTooManyRequests, "M_LIMIT_EXCEEDED", map[string]any{"retry_after_ms": float64(1500)}, nil),
			wantStatus: constants.MatrixSendFailedSend, wantRetry: true, wantWait: 1500 * time.Millisecond,
		},
		{
			name:       "rate limited, saying so in a header",
			err:        httpFailure(http.StatusTooManyRequests, "", nil, http.Header{"Retry-After": []string{"3"}}),
			wantStatus: constants.MatrixSendFailedSend, wantRetry: true, wantWait: 3 * time.Second,
		},
		{
			name:       "rate limited, without saying for how long",
			err:        httpFailure(http.StatusTooManyRequests, "M_LIMIT_EXCEEDED", nil, nil),
			wantStatus: constants.MatrixSendFailedSend, wantRetry: true,
		},
		{
			name:       "not allowed to send",
			err:        httpFailure(http.StatusForbidden, "M_FORBIDDEN", nil, nil),
			wantStatus: constants.MatrixSendFailedForbidden,
		},
		{
			name:       "a request the homeserver won't take",
			err:        httpFailure(http.StatusBadRequest, "M_BAD_JSON", nil, nil),
			wantStatus: constants.MatrixSendFailedSend,
		},
		{
			name:       "the deadline cut it short",
			err:        mautrix.HTTPError{Message: "request error", WrappedError: &url.Error{Op: "Post", URL: "https://example.com", Err: context.DeadlineExceeded}},
			wantStatus: constants.MatrixSendTimedOut,
		},
		{
			name:       "something else entirely",
			err:        errors.New("failed to marshal"),
			wantStatus: constants.MatrixSendFailedSend,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, retry, wait := sendFailure(tt.err)
			if status != tt.wantStatus || retry != tt.wantRetry || wait != tt.wantWait {
				t.Errorf("sendFailure() = %s, %v, %v, want %s, %v, %v", status, retry, wait, tt.wantStatus, tt.wantRetry, tt.wantWait)
			}
		})
	}
}

// failingAttempts is an attempt that fails the given number of times before it succeeds, counting
// its calls
func failingAttempts(failures int, retry bool, wait time.Duration, calls *int) func(context.Context) (constants.MatrixSendStatus, bool, time.Duration) {
	return func(context.Context) (constants.MatrixSendStatus, bool, time.Duration) {
		*calls++
		if *calls > failures {
			return constants.MatrixSendSuccess, false, 0
		}
		return constants.MatrixSendFailedSend, retry, wait
	}
}

func TestWithRetriesRetriesUntilItWorks(t *testing.T) {
	calls := 0
	evt := outboundEvent{deadline: time.Now().Add(time.Minute)}

	status := withRetries(context.Background(), evt, failingAttempts(2, true, time.Millisecond, &calls))
	if status != constants.MatrixSendSuccess || calls != 3 {
		t.Errorf("withRetries() = %s after %d attempts, want success after 3", status, calls)
	}
}

func TestWithRetriesStopsAtAFailureNotWorthRetrying(t *testing.T) {
	calls := 0
	evt := outboundEvent{deadline: time.Now().Add(time.Minute)}

	status := withRetries(context.Background(), evt, failingAttempts(5, false, 0, &calls))
	if status != constants.MatrixSendFailedSend || calls != 1 {
		t.Errorf("withRetries() = %s after %d attempts, want the failure after 1", status, calls)
	}
}

// An attempt that would come after the deadline isn't made, and the message is given up at once
// rather than after waiting for it
func TestWithRetriesGivesUpAtTheDeadline(t *testing.T) {
	calls := 0
	evt := outboundEvent{deadline: time.Now().Add(100 * time.Millisecond)}

	start := time.Now()
	status := withRetries(context.Background(), evt, failingAttempts(5, true, time.Second, &calls))
	if status != constants.MatrixSendTimedOut || calls != 1 {
		t.Errorf("withRetries() = %s after %d attempts, want timed out after 1", status, calls)
	}
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Errorf("giving up took %v", elapsed)
	}
}

// A message whose trigger was deleted while its send was being retried isn't sent after all
func TestWithRetriesDropsAMessageNoLongerWanted(t *testing.T) {
	calls := 0
	wanted := true
	evt := outboundEvent{
		deadline: time.Now().Add(time.Minute),
		target:   &Target{Cancelled: func() bool { return !wanted }},
	}

	status := withRetries(context.Background(), evt, func(ctx context.Context) (constants.MatrixSendStatus, bool, time.Duration) {
		calls++
		wanted = false
		return constants.MatrixSendFailedSend, true, time.Millisecond
	})
	if status != constants.MatrixSendDropped || calls != 1 {
		t.Errorf("withRetries() = %s after %d attempts, want dropped after 1", status, calls)
	}
}

func TestNewOutboundEventDeadline(t *testing.T) {
	before := time.Now()

	plain := newOutboundEvent(context.Background(), testRoomID, simpleMessage{Body: "pong"}, nil, nil)
	if plain.deadline.Before(before.Add(defaultSendTimeout)) || plain.deadline.After(time.Now().Add(defaultSendTimeout)) {
		t.Errorf("an untargeted message's deadline is %v from now, want %v", time.Until(plain.deadline), defaultSendTimeout)
	}

	awaited := newOutboundEvent(context.Background(), testRoomID, simpleMessage{Body: "sunny"}, &Target{Timeout: 30 * time.Second}, nil)
	if awaited.deadline.After(time.Now().Add(30 * time.Second)) {
		t.Errorf("a message with a timeout has a deadline %v from now, want at most 30s", time.Until(awaited.deadline))
	}
}

package aigateway

import (
	"context"
	"testing"
	"time"
)

// TestIsRetryableOnlyCoversTransientFailures guards the property that matters most here: a request
// that is wrong will be just as wrong the second time, and retrying it wastes a user's wait and an
// inference charge.
func TestIsRetryableOnlyCoversTransientFailures(t *testing.T) {
	retryable := []ErrorKind{
		ErrorKindNetwork,
		ErrorKindTimeout,
		ErrorKindRateLimited,
		ErrorKindProviderError,
	}
	permanent := []ErrorKind{
		ErrorKindAuth,
		ErrorKindModelNotFound,
		ErrorKindBadRequest,
		ErrorKindParseError,
		ErrorKindCancelled,
		ErrorKindUnknown,
	}

	for _, kind := range retryable {
		if !isRetryable(kind) {
			t.Errorf("%q should be retried", kind)
		}
	}
	for _, kind := range permanent {
		if isRetryable(kind) {
			t.Errorf("%q must not be retried", kind)
		}
	}
}

// TestWaitBeforeRetryStopsOnCancelledContext verifies a retry never outlives the turn that wanted it
func TestWaitBeforeRetryStopsOnCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	if waitBeforeRetry(ctx, 1) {
		t.Error("expected the wait to report failure on a cancelled context")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("expected an immediate return, waited %s", elapsed)
	}
}

// TestRetryDelayBacksOffWithinBounds verifies the delay grows with the attempt number and stays
// inside the ceiling, jitter included. Tests the computation rather than the sleep, so it is fast.
func TestRetryDelayBacksOffWithinBounds(t *testing.T) {
	ceiling := time.Duration(float64(retryMaxDelay) * 1.5)

	// Jitter spans 0.5x to 1.5x of the nominal delay, so compare bounds rather than exact values
	for attempt := 1; attempt <= 8; attempt++ {
		nominal := min(retryBaseDelay<<(attempt-1), retryMaxDelay)
		lower := time.Duration(float64(nominal) * 0.5)
		upper := time.Duration(float64(nominal) * 1.5)

		for range 50 {
			delay := retryDelay(attempt)
			if delay < lower || delay > upper {
				t.Fatalf("attempt %d: delay %s outside [%s, %s]", attempt, delay, lower, upper)
			}
			if delay > ceiling {
				t.Fatalf("attempt %d: delay %s above the ceiling of %s", attempt, delay, ceiling)
			}
		}
	}

	// Later attempts must wait longer, up to the ceiling
	early := time.Duration(float64(min(retryBaseDelay, retryMaxDelay)) * 1.5)
	if retryDelay(4) <= early && retryBaseDelay<<3 <= retryMaxDelay {
		t.Error("expected a later attempt to back off further than the first")
	}
}

// TestWorstCaseCall checks the longest a request can take: every attempt running out of time, and
// the longest wait before each retry, which is one and a half times its nominal delay
func TestWorstCaseCall(t *testing.T) {
	for _, tt := range []struct {
		timeout  time.Duration
		attempts int
		want     time.Duration
	}{
		// A single attempt never waits
		{45 * time.Second, 1, 45 * time.Second},
		// What the constants this replaced added up to: three attempts of 45 seconds, and waits of at
		// most 0.75 and 1.5 seconds
		{45 * time.Second, 3, 135*time.Second + 2250*time.Millisecond},
		// The waits double up to their ceiling of 4 seconds, which is 6 with the jitter
		{10 * time.Second, 5, 50*time.Second + 750*time.Millisecond + 1500*time.Millisecond + 3*time.Second + 6*time.Second},
	} {
		if got := WorstCaseCall(tt.timeout, tt.attempts); got != tt.want {
			t.Errorf("WorstCaseCall(%s, %d) = %s, want %s", tt.timeout, tt.attempts, got, tt.want)
		}
	}

	// No wait is longer than the worst case allows for it, so a caller that allows the worst case
	// always has room for the retries
	for attempt := 1; attempt <= 8; attempt++ {
		allowed := WorstCaseCall(0, attempt+1) - WorstCaseCall(0, attempt)
		for range 50 {
			if delay := retryDelay(attempt); delay > allowed {
				t.Fatalf("the wait after attempt %d was %s, more than the %s the worst case allows", attempt, delay, allowed)
			}
		}
	}
}

// TestHTTPTimeoutIsBackstopOnly verifies the client timeout sits above the longest attempt a request
// may be given, so the per-attempt context is what actually fires and the error classifies as a
// timeout
func TestHTTPTimeoutIsBackstopOnly(t *testing.T) {
	if httpTimeout <= MaxCallTimeout {
		t.Errorf("httpTimeout (%s) must be above MaxCallTimeout (%s) or it pre-empts the per-attempt deadline",
			httpTimeout, MaxCallTimeout)
	}
}

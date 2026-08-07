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

// TestRetryBudgetFitsInsideTurn checks the timeouts compose: the worst case for a single call,
// retries and backoff included, has to leave room inside a turn for more than one call.
func TestRetryBudgetFitsInsideTurn(t *testing.T) {
	// Mirrors the turn deadline in the chat package. Kept as a literal on purpose, so that changing
	// one side without the other trips this test rather than silently making retries unreachable.
	const turnTimeout = 3 * time.Minute

	worstCaseBackoff := time.Duration(0)
	for attempt := 1; attempt < maxAttempts; attempt++ {
		delay := retryBaseDelay << (attempt - 1)
		if delay > retryMaxDelay {
			delay = retryMaxDelay
		}
		worstCaseBackoff += time.Duration(float64(delay) * 1.5)
	}

	worstCaseCall := time.Duration(maxAttempts)*callTimeout + worstCaseBackoff
	if worstCaseCall >= turnTimeout {
		t.Errorf("a single fully retried call can take %s, which exceeds the %s turn budget",
			worstCaseCall, turnTimeout)
	}
}

// TestHTTPTimeoutIsBackstopOnly verifies the client timeout sits above the per-attempt deadline, so
// the per-attempt context is what actually fires and the error classifies as a timeout
func TestHTTPTimeoutIsBackstopOnly(t *testing.T) {
	if httpTimeout <= callTimeout {
		t.Errorf("httpTimeout (%s) must be above callTimeout (%s) or it pre-empts the per-attempt deadline",
			httpTimeout, callTimeout)
	}
}

// TestGatewayTimeoutMatchesCallTimeout verifies the gateway is asked to give up when the client does
func TestGatewayTimeoutMatchesCallTimeout(t *testing.T) {
	if want := int(callTimeout / time.Millisecond); gatewayTimeoutMS != want {
		t.Errorf("gatewayTimeoutMS = %d, want %d", gatewayTimeoutMS, want)
	}
}

package chat

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// TestTypingIndicatorRefreshesBeforeItExpires verifies the refresh interval sits inside the timeout
// it renews. If they were the other way round the indicator would lapse between refreshes, which is
// the bug this replaced: the indicator was set once per iteration, so a long iteration left the bot
// looking idle while it was still working.
func TestTypingIndicatorRefreshesBeforeItExpires(t *testing.T) {
	if typingIndicatorRefresh >= typingIndicatorTimeout {
		t.Errorf("refresh interval %s must be shorter than the timeout %s it renews",
			typingIndicatorRefresh, typingIndicatorTimeout)
	}

	// Leave real headroom rather than refreshing at the last possible moment, since the refresh has
	// to survive a slow round trip to the homeserver
	if margin := typingIndicatorTimeout - typingIndicatorRefresh; margin < 5*time.Second {
		t.Errorf("only %s of margin between refresh and expiry, which is too tight to absorb a slow request", margin)
	}
}

// TestKeepAliveRefreshesUntilStopped verifies the indicator is actually renewed while a turn runs,
// which is the whole point: it previously lapsed whenever an iteration outlived its timeout
func TestKeepAliveRefreshesUntilStopped(t *testing.T) {
	var refreshes atomic.Int32
	var stops atomic.Int32

	stop := keepAlive(context.Background(), 10*time.Millisecond,
		func() { refreshes.Add(1) },
		func() { stops.Add(1) })

	time.Sleep(120 * time.Millisecond)
	stop()

	if got := refreshes.Load(); got < 3 {
		t.Errorf("expected several refreshes over the interval, got %d", got)
	}
	if got := stops.Load(); got != 1 {
		t.Errorf("expected exactly one stop, got %d", got)
	}

	// No further refreshes once stopped
	settled := refreshes.Load()
	time.Sleep(50 * time.Millisecond)
	if got := refreshes.Load(); got != settled {
		t.Errorf("refreshes continued after stopping: %d then %d", settled, got)
	}
}

// TestKeepAliveStopIsIdempotent verifies stopping twice is safe. The stop runs from a defer while
// the error paths return early, so a double stop is easy to reach and must not panic on a closed
// channel or fire the stop action twice.
func TestKeepAliveStopIsIdempotent(t *testing.T) {
	var stops atomic.Int32

	stop := keepAlive(context.Background(), time.Hour, func() {}, func() { stops.Add(1) })

	stop()
	stop()
	stop()

	if got := stops.Load(); got != 1 {
		t.Errorf("expected the stop action to run once, ran %d times", got)
	}
}

// TestKeepAliveStopsWithTheContext verifies the refresh goroutine exits when the turn's context
// ends, rather than outliving the turn it belongs to
func TestKeepAliveStopsWithTheContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	var refreshes atomic.Int32
	stop := keepAlive(ctx, 10*time.Millisecond, func() { refreshes.Add(1) }, func() {})
	defer stop()

	cancel()
	time.Sleep(50 * time.Millisecond)

	settled := refreshes.Load()
	time.Sleep(50 * time.Millisecond)
	if got := refreshes.Load(); got != settled {
		t.Errorf("refreshes continued after the context ended: %d then %d", settled, got)
	}
}

// TestTurnBudgetExceedsASingleCall verifies the turn deadline leaves room for more than one model
// call, so a turn is not cut off before it can use a tool result
func TestTurnBudgetExceedsASingleCall(t *testing.T) {
	// Mirrors aigateway.callTimeout. Kept as a literal because importing that package here would be
	// a cycle; the aigateway side has the matching check against the turn budget.
	const callTimeout = 45 * time.Second

	if turnTimeout <= 2*callTimeout {
		t.Errorf("turn budget %s leaves no room for a tool loop on top of a %s call",
			turnTimeout, callTimeout)
	}
}

// TestPersistTimeoutIsShort verifies the write that records a turn cannot itself hold things up for
// long, since it runs on a context deliberately detached from the turn's cancellation
func TestPersistTimeoutIsShort(t *testing.T) {
	if persistTimeout > 15*time.Second {
		t.Errorf("persist timeout %s is long enough to delay a turn's completion", persistTimeout)
	}
}

// TestPersistContextSurvivesCancellation is the property that makes the accounting trustworthy: a
// turn that timed out is exactly the one whose record matters, so its writes must still go through.
func TestPersistContextSurvivesCancellation(t *testing.T) {
	turnCtx, cancel := context.WithCancel(context.Background())
	cancel()

	if turnCtx.Err() == nil {
		t.Fatal("expected the turn context to be cancelled")
	}

	persistCtx, cancelPersist := persistContext(turnCtx)
	defer cancelPersist()

	if err := persistCtx.Err(); err != nil {
		t.Errorf("persist context inherited the cancellation: %v", err)
	}

	if _, ok := persistCtx.Deadline(); !ok {
		t.Error("expected the persist context to carry its own deadline")
	}
}

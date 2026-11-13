package remind

import (
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

// delivery is what a reminder's timer does: claim it, and send it only if the claim succeeds
func delivery(s *schedule, id int64, sent *atomic.Bool) func() {
	return func() {
		if s.claim(id) {
			sent.Store(true)
		}
	}
}

func TestACancelledReminderIsNeverSent(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := newSchedule()
		var sent atomic.Bool
		s.add(1, time.Hour, delivery(s, 1, &sent))

		if !s.cancel(1) {
			t.Fatal("cancelling a waiting reminder failed")
		}
		time.Sleep(2 * time.Hour)
		synctest.Wait()

		if sent.Load() {
			t.Error("a cancelled reminder was sent")
		}
	})
}

func TestAReminderSentFirstCantBeCancelled(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := newSchedule()
		var sent atomic.Bool
		s.add(1, time.Hour, delivery(s, 1, &sent))

		time.Sleep(2 * time.Hour)
		synctest.Wait()

		if !sent.Load() {
			t.Fatal("the reminder wasn't sent")
		}
		if s.cancel(1) {
			t.Error("a reminder that was already sent was cancelled")
		}
	})
}

// A reminder restored at startup after its time goes off at once
func TestAnOverdueReminderIsSentAtOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := newSchedule()
		var sent atomic.Bool
		s.add(1, -time.Minute, delivery(s, 1, &sent))
		synctest.Wait()

		if !sent.Load() {
			t.Error("an overdue reminder wasn't sent at once")
		}
	})
}

func TestClaimAndCancelEachWinOnce(t *testing.T) {
	s := newSchedule()
	s.add(1, time.Hour, func() {})
	if !s.cancel(1) || s.claim(1) || s.cancel(1) {
		t.Error("a reminder could be claimed or cancelled after it was cancelled")
	}

	s.add(2, time.Hour, func() {})
	if !s.claim(2) || s.claim(2) || s.cancel(2) {
		t.Error("a reminder could be claimed or cancelled after it was claimed")
	}
	if s.claim(3) || s.cancel(3) {
		t.Error("a reminder that was never scheduled could be claimed or cancelled")
	}
}

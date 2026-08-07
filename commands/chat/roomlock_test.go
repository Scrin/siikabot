package chat

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestLockRoomSerialisesSameRoom verifies two turns in one room never overlap. Overlapping turns
// interleave history writes and let the first one to finish switch off the typing indicator while
// the other is still working.
func TestLockRoomSerialisesSameRoom(t *testing.T) {
	const goroutines = 8

	var inFlight atomic.Int32
	var maxInFlight atomic.Int32
	var wg sync.WaitGroup

	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()

			unlock := lockRoom("!serialised:example.org")
			defer unlock()

			current := inFlight.Add(1)
			for {
				observed := maxInFlight.Load()
				if current <= observed || maxInFlight.CompareAndSwap(observed, current) {
					break
				}
			}
			time.Sleep(time.Millisecond)
			inFlight.Add(-1)
		}()
	}

	wg.Wait()

	if got := maxInFlight.Load(); got != 1 {
		t.Errorf("expected at most 1 concurrent turn per room, saw %d", got)
	}
}

// TestLockRoomDoesNotBlockOtherRooms verifies the lock is per room, so a slow turn in one room
// cannot stall every other room.
func TestLockRoomDoesNotBlockOtherRooms(t *testing.T) {
	unlockFirst := lockRoom("!first:example.org")
	defer unlockFirst()

	done := make(chan struct{})
	go func() {
		unlock := lockRoom("!second:example.org")
		unlock()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("a locked room blocked an unrelated room")
	}
}

// TestLockRoomIsReusable verifies the same room can be locked again after release, i.e. the stored
// mutex is reused rather than replaced.
func TestLockRoomIsReusable(t *testing.T) {
	unlock := lockRoom("!reused:example.org")
	unlock()

	done := make(chan struct{})
	go func() {
		unlock := lockRoom("!reused:example.org")
		unlock()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("re-locking a released room deadlocked")
	}
}

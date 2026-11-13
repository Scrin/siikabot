package remind

import (
	"sync"
	"time"
)

// schedule keeps the timers of the reminders that are waiting to go off, so that a reminder deleted
// before it goes off can be stopped. The timer holds its own copy of the reminder, so deleting the
// row alone wouldn't stop it.
//
// A timer that fires claims its reminder first, and a deletion cancels it. Whichever comes first
// wins, so a reminder is either sent or deleted, never both.
type schedule struct {
	mu     sync.Mutex
	timers map[int64]*time.Timer
}

func newSchedule() *schedule {
	return &schedule{timers: make(map[int64]*time.Timer)}
}

// scheduled holds the reminders waiting to go off
var scheduled = newSchedule()

// add starts a timer that calls f after d, or at once if d isn't positive. f should claim the
// reminder before doing anything.
func (s *schedule) add(id int64, d time.Duration, f func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.timers[id] = time.AfterFunc(max(d, 0), f)
}

// claim takes the reminder out of the schedule for sending. It reports false if it was cancelled
// or claimed already.
func (s *schedule) claim(id int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.timers[id]
	delete(s.timers, id)
	return ok
}

// cancel stops the reminder's timer. It reports false if the reminder wasn't waiting to go off,
// because its timer has already claimed it or it was never scheduled.
func (s *schedule) cancel(id int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	timer, ok := s.timers[id]
	if !ok {
		return false
	}
	timer.Stop()
	delete(s.timers, id)
	return true
}

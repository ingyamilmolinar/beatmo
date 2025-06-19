package beat

import "time"

// SetNowFunc is **only** compiled when the “test” build-tag is active.
// The UI test-suite uses it to deterministically advance the scheduler’s clock.
func (s *Scheduler) SetNowFunc(f func() time.Time) {
	s.now = f
}


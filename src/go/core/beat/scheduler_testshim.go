package beat

import "time"

// A deterministic clock, so the scheduler advances in lock-step with assertions.
func (s *Scheduler) SetNowFunc(f func() time.Time) {
	s.now = f
}

package beat

import "time"

// SetNowFunc lets tests inject a deterministic clock so the scheduler
// advances in lock-step with assertions instead of wall time. The
// previously-defined GraphInterface / MockGraph types in this file were
// dead code (no internal or external consumer); they have been removed.
func (s *Scheduler) SetNowFunc(f func() time.Time) {
	s.now = f
}

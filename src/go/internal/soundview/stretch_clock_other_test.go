//go:build !linux

package soundview

import "time"

// stretchClock falls back to the wall clock where no per-thread CPU clock is
// wired up (see the linux file): it also counts OS preemption.
func stretchClock() (now func() time.Duration, release func()) {
	start := time.Now()
	return func() time.Duration { return time.Since(start) }, func() {}
}

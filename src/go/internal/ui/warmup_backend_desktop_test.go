//go:build test && !js

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/async"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// TestDesktopWarmBackendNeverDropsWork pins the non-dropping contract: the
// gated warm set must be fed with SubmitBlocking, never Submit. Submit drops on
// backpressure by design (ErrBackpressure + a drop counter), which is exactly
// how 273 warm requests into a 64-deep queue silently lost most of the working
// set before this change.
func TestDesktopWarmBackendNeverDropsWork(t *testing.T) {
	const poolName = "test.warmup.nodrop"
	pool, err := async.DefaultRegistry().Get(poolName, async.Options{
		MaxConcurrent: 1, QueueSize: 2, Name: poolName,
	})
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(func() { async.DefaultRegistry().Release(poolName) })

	be := newDesktopWarmBackend(pool)
	const n = 64 // 32x the queue depth
	done := make(chan struct{}, n)
	for i := 0; i < n; i++ {
		be.Warm(roster.Kick, float64(i)/2, func() { done <- struct{}{} })
	}
	for i := 0; i < n; i++ {
		<-done // blocks forever (test timeout) if any item was dropped
	}
	if d := pool.Stats().Dropped; d != 0 {
		t.Errorf("pool dropped %d jobs; the gated warm set must never drop", d)
	}
}

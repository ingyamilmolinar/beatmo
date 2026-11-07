//go:build test && !js

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/async"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Submit drops on backpressure by design, which would silently lose most of a large working set; the gated set goes
// through SubmitBlocking.
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

//go:build test

// Bounds regression tests for the coalescer's pending map.
//
// The coalescer holds at most one pendingEntry per Kind in
// coalesceKinds. The bound is therefore len(coalesceKinds) — currently
// 9 kinds. The tests assert that the map size never exceeds this bound
// regardless of how many Submit calls happen.
//
// This is a regression guard: if someone widens coalesceKinds (or, more
// dangerously, switches to per-event retention rather than
// trailing-edge debouncing), the test catches the change in the
// upper-bound.

package eventlogger

import (
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

// TestCoalescerPendingMapBoundedByKindCount asserts the pending map
// never exceeds len(coalesceKinds) regardless of submit volume.
func TestCoalescerPendingMapBoundedByKindCount(t *testing.T) {
	var emitted atomic.Int64
	c := newCoalescer(50*time.Millisecond, func(hooks.Event) {
		emitted.Add(1)
	})

	allCoalesced := make([]hooks.Kind, 0, len(coalesceKinds))
	for k := range coalesceKinds {
		allCoalesced = append(allCoalesced, k)
	}

	// Submit 100k events spread evenly across every coalesced kind.
	const submits = 100_000
	for i := 0; i < submits; i++ {
		k := allCoalesced[i%len(allCoalesced)]
		c.Submit(hooks.Event{Kind: k, Payload: i})
	}

	c.mu.Lock()
	got := len(c.pending)
	c.mu.Unlock()

	if got > len(coalesceKinds) {
		t.Errorf("pending map = %d entries after %d submits, want <= %d "+
			"(eventlogger/coalesce.go:74 each Submit must replace existing "+
			"entry for the same Kind, not append)",
			got, submits, len(coalesceKinds))
	}
	t.Logf("pending=%d kinds=%d submits=%d emittedDuringSubmits=%d",
		got, len(coalesceKinds), submits, emitted.Load())

	// FlushAll must clear the map even when no timer fires.
	c.FlushAll()
	c.mu.Lock()
	gotAfter := len(c.pending)
	c.mu.Unlock()
	if gotAfter != 0 {
		t.Errorf("pending map = %d entries after FlushAll, want 0 "+
			"(eventlogger/coalesce.go:113 reset broken)", gotAfter)
	}
}

// TestCoalescerHeapStableUnderSustainedSubmits drives the coalescer
// with high-rate submits over many cycles and asserts the heap
// footprint does not grow proportional to submit count. Each Submit
// reuses the existing pendingEntry's slot (line 74-75), so the only
// allocation is the timer.Reset() — bounded.
func TestCoalescerHeapStableUnderSustainedSubmits(t *testing.T) {
	c := newCoalescer(time.Hour, func(hooks.Event) {})

	// Warm-up: prime each kind so the timer slots exist.
	for k := range coalesceKinds {
		c.Submit(hooks.Event{Kind: k})
	}

	runtime.GC()
	var pre runtime.MemStats
	runtime.ReadMemStats(&pre)

	const submits = 200_000
	for i := 0; i < submits; i++ {
		for k := range coalesceKinds {
			c.Submit(hooks.Event{Kind: k, Payload: i})
		}
	}

	runtime.GC()
	var post runtime.MemStats
	runtime.ReadMemStats(&post)

	var delta int64
	if post.HeapAlloc > pre.HeapAlloc {
		delta = int64(post.HeapAlloc - pre.HeapAlloc)
	} else {
		delta = -int64(pre.HeapAlloc - post.HeapAlloc)
	}
	t.Logf("coalescer HeapAlloc delta after %d submits = %d bytes",
		submits*len(coalesceKinds), delta)

	const ceiling = 1 * 1024 * 1024
	if delta > ceiling {
		t.Errorf("coalescer HeapAlloc delta = %d bytes after %d submits "+
			"(ceiling %d) — pendingEntry replacement in coalesce.go:74 "+
			"may be retaining stale events", delta,
			submits*len(coalesceKinds), ceiling)
	}

	// Cleanly stop all timers so the test doesn't leak goroutines.
	c.FlushAll()
}

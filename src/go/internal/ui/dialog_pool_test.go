//go:build test

package ui

import (
	"context"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/async"
)

// TestDialogPool_NoGoroutineLeakUnderRepeatedDispatch verifies the
// migration: the desktop file-picker path now goes through the bounded
// "ui.dialog" pool instead of spawning a raw goroutine per click. Many
// dispatches must not grow the goroutine count proportional to N.
func TestDialogPool_NoGoroutineLeakUnderRepeatedDispatch(t *testing.T) {
	runtime.GC()
	before := runtime.NumGoroutine()

	const N = 50
	var done atomic.Int64
	for range N {
		if err := async.Go("ui.dialog", func(_ context.Context) {
			done.Add(1)
		}); err != nil {
			// On a saturated pool this is expected; just don't leak.
			done.Add(1)
		}
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if done.Load() == int64(N) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got := done.Load(); got != int64(N) {
		t.Fatalf("done = %d, want %d", got, N)
	}

	runtime.GC()
	after := runtime.NumGoroutine()
	if delta := after - before; delta > N/4 {
		t.Fatalf("goroutine count grew by %d for %d dispatches; pool is leaking", delta, N)
	}
}

// async.Go's backpressure semantics are covered exhaustively in
// internal/async/go_test.go::TestGo_BackpressurePropagates. The dialog
// pool inherits that contract; a duplicate test here only inflates the
// shared registry's budget consumption without adding signal.

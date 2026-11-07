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

// Through the bounded "ui.dialog" pool: many dispatches must not grow the goroutine count.
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

// Backpressure is async.Go's contract and tested there; a copy here would only spend the shared budget.

package engine

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/core/model"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

func newTestPredictor(t *testing.T) *Predictor {
	t.Helper()
	logger := game_log.New(nil, game_log.LevelError)
	g := model.NewGraph(logger)
	return NewPredictor(g, logger)
}

// TestStartBackground_DoubleStartDoesNotLeakGoroutine confirms the CAS
// gate works: many calls to StartBackground spawn at most one worker.
func TestStartBackground_DoubleStartDoesNotLeakGoroutine(t *testing.T) {
	p := newTestPredictor(t)

	runtime.GC()
	before := runtime.NumGoroutine()

	for range 10 {
		p.StartBackground(func() int { return 0 })
	}
	// Let the worker (if any) settle.
	time.Sleep(20 * time.Millisecond)
	peak := runtime.NumGoroutine()
	if delta := peak - before; delta > 2 {
		t.Fatalf("goroutine count grew by %d after 10 StartBackground calls (before=%d peak=%d); expected ≤ 2",
			delta, before, peak)
	}

	p.StopBackground()
	time.Sleep(20 * time.Millisecond)
	after := runtime.NumGoroutine()
	if delta := after - before; delta > 1 {
		t.Fatalf("goroutine count after Stop = %d (before=%d); expected ≤ 1", delta, after-before)
	}
}

// TestStartBackground_StopAndRestart verifies the legitimate cycle
// works: Start → Stop → Start spawns a fresh worker that runs targetFn.
func TestStartBackground_StopAndRestart(t *testing.T) {
	p := newTestPredictor(t)

	var calls atomic.Int64
	target := func() int {
		calls.Add(1)
		return 0
	}

	p.StartBackground(target)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if calls.Load() >= 2 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if calls.Load() < 2 {
		t.Fatalf("first cycle: target invoked %d times, want ≥ 2", calls.Load())
	}

	p.StopBackground()
	stopMark := calls.Load()
	time.Sleep(30 * time.Millisecond)
	if drift := calls.Load() - stopMark; drift > 0 {
		t.Fatalf("target invoked %d more times after Stop; worker did not exit", drift)
	}

	// Second cycle.
	p.StartBackground(target)
	t.Cleanup(p.StopBackground)
	deadline = time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if calls.Load()-stopMark >= 2 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if calls.Load()-stopMark < 2 {
		t.Fatalf("second cycle: target invoked %d times, want ≥ 2 after restart", calls.Load()-stopMark)
	}
}

// TestStartBackground_ConcurrentStartIsSafe fans out concurrent Start
// calls and asserts at most one goroutine is spawned. The CAS gate
// linearizes the race.
func TestStartBackground_ConcurrentStartIsSafe(t *testing.T) {
	p := newTestPredictor(t)
	t.Cleanup(p.StopBackground)

	runtime.GC()
	before := runtime.NumGoroutine()

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.StartBackground(func() int { return 0 })
		}()
	}
	wg.Wait()
	time.Sleep(20 * time.Millisecond)

	peak := runtime.NumGoroutine()
	if delta := peak - before; delta > 2 {
		t.Fatalf("concurrent Start spawned %d goroutines (before=%d peak=%d); expected ≤ 2",
			delta, before, peak)
	}
}

// TestStartBackground_TargetUpdatesOnRepeatCall verifies that calling
// StartBackground a second time with a different target replaces the
// target function (without spawning a second worker).
func TestStartBackground_TargetUpdatesOnRepeatCall(t *testing.T) {
	p := newTestPredictor(t)
	t.Cleanup(p.StopBackground)

	var calledA, calledB atomic.Int64
	p.StartBackground(func() int { calledA.Add(1); return 0 })
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if calledA.Load() >= 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if calledA.Load() == 0 {
		t.Fatal("first target never invoked")
	}

	p.StartBackground(func() int { calledB.Add(1); return 0 })
	deadline = time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if calledB.Load() >= 2 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if calledB.Load() < 2 {
		t.Fatalf("second target invoked %d times; expected ≥ 2 after replacement", calledB.Load())
	}
}

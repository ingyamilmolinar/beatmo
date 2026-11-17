package engine

import (
	"sync/atomic"
	"testing"
)

// TestStopBackgroundOnNeverStartedIsNoOp covers the documented "safe to
// call when no worker is running" branch of StopBackground — exercising
// the `quit == nil` and `done == nil` paths that the start-stop happy
// case never reaches.
func TestStopBackgroundOnNeverStartedIsNoOp(t *testing.T) {
	p := newTestPredictor(t)
	// No StartBackground was ever called.
	p.StopBackground()
	// And it must remain idempotent.
	p.StopBackground()
}

// TestStopBackgroundClearsCASGate proves the "Start → Stop → Start"
// cycle works because Stop clears bgRunning AFTER the worker has
// returned. We don't call StartBackground (and a real worker), only
// StopBackground twice — the second call hits the bgRunning=false path.
func TestStopBackgroundClearsCASGate(t *testing.T) {
	p := newTestPredictor(t)
	p.StartBackground(func() int { return 0 })
	p.StopBackground()
	if p.bgRunning.Load() {
		t.Error("bgRunning still true after Stop; second Start would be blocked")
	}
}

// TestSetBackgroundTarget_NoWorkerSpawn covers the test seam used by
// callers who want to evaluate target logic without spinning a
// goroutine. After SetBackgroundTarget, BackgroundTargetForTest returns
// the same function pointer.
func TestSetBackgroundTarget_NoWorkerSpawn(t *testing.T) {
	p := newTestPredictor(t)
	var calls atomic.Int64
	target := func() int {
		calls.Add(1)
		return 42
	}
	p.SetBackgroundTarget(target)
	got := p.BackgroundTargetForTest()
	if got == nil {
		t.Fatal("BackgroundTargetForTest returned nil after SetBackgroundTarget")
	}
	if v := got(); v != 42 {
		t.Errorf("target() returned %d, want 42", v)
	}
	if calls.Load() != 1 {
		t.Errorf("target invoked %d times by the test seam, want exactly 1", calls.Load())
	}
	if p.bgRunning.Load() {
		t.Error("SetBackgroundTarget incorrectly spawned a worker (bgRunning=true)")
	}
}

// TestSetBackgroundTargetReplacesPrevious covers the overwrite path:
// calling SetBackgroundTarget twice swaps the function pointer
// in-place.
func TestSetBackgroundTargetReplacesPrevious(t *testing.T) {
	p := newTestPredictor(t)
	p.SetBackgroundTarget(func() int { return 1 })
	p.SetBackgroundTarget(func() int { return 2 })
	tf := p.BackgroundTargetForTest()
	if tf == nil {
		t.Fatal("BackgroundTargetForTest returned nil after second SetBackgroundTarget")
	}
	if v := tf(); v != 2 {
		t.Errorf("BackgroundTargetForTest()() = %d, want 2 (the second target)", v)
	}
}

// TestHorizonReturnsWindowEnd is the unit-level guard for the public
// horizon accessor. A fresh predictor has windowEnd == 0; after Ensure
// it advances.
func TestHorizonReturnsWindowEnd(t *testing.T) {
	p := newTestPredictor(t)
	if h := p.Horizon(); h != 0 {
		t.Errorf("fresh predictor Horizon()=%d, want 0", h)
	}
	p.Ensure(64)
	if h := p.Horizon(); h < 64 {
		t.Errorf("after Ensure(64): Horizon()=%d, want ≥ 64", h)
	}
}

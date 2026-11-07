package engine

import (
	"sync/atomic"
	"testing"
)

// The quit == nil / done == nil paths the start-stop case never reaches.
func TestStopBackgroundOnNeverStartedIsNoOp(t *testing.T) {
	p := newTestPredictor(t)
	// No StartBackground was ever called.
	p.StopBackground()
	// And it must remain idempotent.
	p.StopBackground()
}

// The second Stop hits the bgRunning=false path, which is what makes Start → Stop → Start work.
func TestStopBackgroundClearsCASGate(t *testing.T) {
	p := newTestPredictor(t)
	p.StartBackground(func() int { return 0 })
	p.StopBackground()
	if p.bgRunning.Load() {
		t.Error("bgRunning still true after Stop; second Start would be blocked")
	}
}

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

package beat

import (
	"math"
	"reflect"
	"testing"
	"time"
)

func TestSchedulerCatchUp(t *testing.T) {
	s := NewScheduler(nil)
	s.BPM = 60 // 1 beat per second
	base := time.Unix(0, 0)
	now := base
	s.now = func() time.Time { return now }
	var steps []int
	s.OnTick = func(step int) {
		steps = append(steps, step)
	}

	s.Start()
	s.Tick()
	if !reflect.DeepEqual(steps, []int{0}) {
		t.Fatalf("expected first tick, got %v", steps)
	}

	// Advance time by 3 beats; Tick should fire three additional times.
	now = base.Add(3 * time.Second)
	s.Tick()
	if !reflect.DeepEqual(steps, []int{0, 1, 2, 3}) {
		t.Fatalf("expected catch-up ticks [0 1 2 3], got %v", steps)
	}
}

func TestSchedulerProgressStopsAfterStop(t *testing.T) {
	s := NewScheduler(nil)
	s.BPM = 60
	base := time.Unix(0, 0)
	now := base
	s.now = func() time.Time { return now }

	s.Start()
	s.Tick()
	now = now.Add(500 * time.Millisecond)
	if got := s.Progress(); math.Abs(got-0.5) > 0.01 {
		t.Fatalf("progress before stop = %v want 0.5", got)
	}
	s.Stop()
	now = now.Add(time.Second)
	if got := s.Progress(); got != 0 {
		t.Fatalf("progress after stop = %v want 0", got)
	}
}

func TestSchedulerSetBPMPreservesPhase(t *testing.T) {
	s := NewScheduler(nil)
	s.BPM = 60
	base := time.Unix(0, 0)
	now := base
	s.now = func() time.Time { return now }

	s.Start()
	s.Tick()
	now = base.Add(500 * time.Millisecond)
	before := s.Progress()
	if math.Abs(before-0.5) > 0.01 {
		t.Fatalf("pre-change progress = %v want ~0.5", before)
	}

	s.SetBPM(120)
	after := s.Progress()
	if math.Abs(after-before) > 0.05 {
		t.Fatalf("progress drift after BPM change: before=%v after=%v", before, after)
	}
}

func TestSchedulerNoTicksWhenBPMNonPositive(t *testing.T) {
	s := NewScheduler(nil)
	s.BPM = 0
	base := time.Unix(0, 0)
	now := base
	s.now = func() time.Time { return now }
	var steps []int
	s.OnTick = func(step int) { steps = append(steps, step) }

	s.Start()
	s.Tick()
	if len(steps) != 0 {
		t.Fatalf("expected no ticks with BPM<=0, got %v", steps)
	}
	if got := s.Progress(); got != 0 {
		t.Fatalf("expected progress=0 with BPM<=0, got %v", got)
	}
}

func TestSchedulerWrapsBeatLength(t *testing.T) {
	s := NewScheduler(nil)
	s.BPM = 60
	s.BeatLength = 4
	base := time.Unix(0, 0)
	now := base
	s.now = func() time.Time { return now }
	var steps []int
	s.OnTick = func(step int) { steps = append(steps, step) }

	s.Start()
	s.Tick()
	now = base.Add(5 * time.Second) // 5 beats
	s.Tick()
	if !reflect.DeepEqual(steps, []int{0, 1, 2, 3, 0, 1}) {
		t.Fatalf("unexpected step wrap: %v", steps)
	}
}

// --- Phase 2: Beat Scheduler Edge Cases ---

func TestSetBPMNonPositive(t *testing.T) {
	s := NewScheduler(nil)
	s.BPM = 120
	s.SetBPM(-10)
	if s.BPM != -10 {
		t.Fatalf("expected BPM=-10 after SetBPM(-10), got %d", s.BPM)
	}

	// With non-positive BPM, Tick should be a no-op (guard at line 74).
	base := time.Unix(0, 0)
	now := base
	s.now = func() time.Time { return now }
	var steps []int
	s.OnTick = func(step int) { steps = append(steps, step) }

	s.Start()
	now = base.Add(time.Second)
	s.Tick()
	if len(steps) != 0 {
		t.Fatalf("expected no ticks with BPM=-10, got %v", steps)
	}

	// SetBPM(0) should also be stored directly.
	s.SetBPM(0)
	if s.BPM != 0 {
		t.Fatalf("expected BPM=0 after SetBPM(0), got %d", s.BPM)
	}
}

func TestSetBPMSameValue(t *testing.T) {
	s := NewScheduler(nil)
	s.BPM = 120
	base := time.Unix(0, 0)
	now := base
	s.now = func() time.Time { return now }

	s.Start()
	s.Tick()                               // Initialize s.last
	now = base.Add(250 * time.Millisecond) // 0.5 beat at 120BPM

	lastBefore := s.last
	s.SetBPM(120) // Same value — the old != bpm guard should skip rebasing.
	lastAfter := s.last

	if lastBefore != lastAfter {
		t.Fatalf("SetBPM with same value should not change s.last: before=%v after=%v", lastBefore, lastAfter)
	}
}

func TestSetBPMBeforeStart(t *testing.T) {
	s := NewScheduler(nil)
	s.BPM = 60
	// s.last is zero because Start() hasn't been called.
	// SetBPM should still update BPM but skip the rebasing block
	// (s.last.IsZero() is true).
	s.SetBPM(90)
	if s.BPM != 90 {
		t.Fatalf("expected BPM=90, got %d", s.BPM)
	}
	// Verify last is still zero (no rebasing happened).
	if !s.last.IsZero() {
		t.Fatalf("expected s.last to remain zero before Start(), got %v", s.last)
	}
}

func TestTickWithoutStart(t *testing.T) {
	s := NewScheduler(nil)
	s.BPM = 120
	base := time.Unix(0, 0)
	now := base
	s.now = func() time.Time { return now }
	var steps []int
	s.OnTick = func(step int) { steps = append(steps, step) }

	// Never call Start(). Tick should be a no-op because running=false.
	now = base.Add(5 * time.Second)
	s.Tick()
	if len(steps) != 0 {
		t.Fatalf("expected no ticks without Start(), got %v", steps)
	}
}

func TestStopThenStartResetsStep(t *testing.T) {
	s := NewScheduler(nil)
	s.BPM = 60
	s.BeatLength = 8
	base := time.Unix(0, 0)
	now := base
	s.now = func() time.Time { return now }
	var steps []int
	s.OnTick = func(step int) { steps = append(steps, step) }

	// Play a few beats.
	s.Start()
	s.Tick() // step 0
	now = base.Add(2 * time.Second)
	s.Tick() // steps 1, 2
	if !reflect.DeepEqual(steps, []int{0, 1, 2}) {
		t.Fatalf("initial run: expected [0 1 2], got %v", steps)
	}

	// Stop, advance time, then start again.
	s.Stop()
	now = now.Add(10 * time.Second) // Time passes while stopped.
	steps = nil

	s.Start()
	s.Tick() // Should fire step 0 (reset), not continue from step 3.
	if len(steps) != 1 || steps[0] != 0 {
		t.Fatalf("after restart: expected [0], got %v", steps)
	}
}

func TestBeatLengthOne(t *testing.T) {
	s := NewScheduler(nil)
	s.BPM = 60
	s.BeatLength = 1
	base := time.Unix(0, 0)
	now := base
	s.now = func() time.Time { return now }
	var steps []int
	s.OnTick = func(step int) { steps = append(steps, step) }

	s.Start()
	s.Tick() // step 0
	now = base.Add(3 * time.Second)
	s.Tick() // steps 0, 0, 0

	// With BeatLength=1, (step+1) % 1 == 0, so every tick fires step 0.
	expected := []int{0, 0, 0, 0}
	if !reflect.DeepEqual(steps, expected) {
		t.Fatalf("BeatLength=1: expected %v, got %v", expected, steps)
	}
}

func TestTickNilOnTick(t *testing.T) {
	s := NewScheduler(nil)
	s.BPM = 60
	base := time.Unix(0, 0)
	now := base
	s.now = func() time.Time { return now }
	s.OnTick = nil // Explicitly nil.

	s.Start()
	// Should not panic even though OnTick is nil.
	s.Tick()
	now = base.Add(3 * time.Second)
	s.Tick()
	// If we reach here, no panic occurred. Verify step advanced correctly.
	// 4 beats elapsed (initial + 3 seconds at 60BPM), step = 4 % 16 = 4.
	if s.currentStep != 4 {
		t.Fatalf("expected currentStep=4 after 4 ticks with nil OnTick, got %d", s.currentStep)
	}
}

func TestTickNoElapsedTime(t *testing.T) {
	s := NewScheduler(nil)
	s.BPM = 60
	base := time.Unix(0, 0)
	now := base
	s.now = func() time.Time { return now }
	var steps []int
	s.OnTick = func(step int) { steps = append(steps, step) }

	s.Start()
	s.Tick() // Fires immediately (first call).
	if !reflect.DeepEqual(steps, []int{0}) {
		t.Fatalf("first tick: expected [0], got %v", steps)
	}

	// Don't advance time. Second Tick should NOT fire OnTick.
	s.Tick()
	if !reflect.DeepEqual(steps, []int{0}) {
		t.Fatalf("tick with zero elapsed: expected [0] (no new tick), got %v", steps)
	}
}

func TestSetBPMFracPreservation(t *testing.T) {
	s := NewScheduler(nil)
	s.BPM = 60 // 1 beat/sec, spb = 1s
	base := time.Unix(0, 0)
	now := base
	s.now = func() time.Time { return now }

	s.Start()
	s.Tick() // Initialize s.last

	// Advance 0.75s → 75% through the beat at 60 BPM.
	now = base.Add(750 * time.Millisecond)
	progBefore := s.Progress()
	if math.Abs(progBefore-0.75) > 0.01 {
		t.Fatalf("pre-change progress: expected ~0.75, got %v", progBefore)
	}

	// Switch to 120 BPM (spb = 0.5s). Progress fraction should be preserved.
	s.SetBPM(120)
	progAfter := s.Progress()
	if math.Abs(progAfter-progBefore) > 0.05 {
		t.Fatalf("progress drift after BPM 60→120: before=%v after=%v", progBefore, progAfter)
	}

	// At 120 BPM, 0.75 fraction of 0.5s = 0.375s remaining.
	// The next tick should fire after 0.25 * 0.5s = 0.125s (since 0.75 * 0.5 = 0.375s elapsed).
	// Actually, let's verify by advancing just past the beat boundary.
	var steps []int
	s.OnTick = func(step int) { steps = append(steps, step) }

	// At 75% through the new beat, we need another 25% = 0.125s to complete.
	now = now.Add(125 * time.Millisecond)
	s.Tick()
	// Should fire exactly one tick (step 1, since step 0 already fired on Start).
	if len(steps) != 1 {
		t.Fatalf("expected 1 tick after completing beat, got %d: %v", len(steps), steps)
	}
}

func TestProgressBPMZero(t *testing.T) {
	s := NewScheduler(nil)
	s.BPM = 60
	base := time.Unix(0, 0)
	now := base
	s.now = func() time.Time { return now }

	s.Start()
	s.Tick()
	now = base.Add(500 * time.Millisecond)

	// Verify we had valid progress first.
	if got := s.Progress(); math.Abs(got-0.5) > 0.01 {
		t.Fatalf("progress at 60BPM: expected ~0.5, got %v", got)
	}

	// Set BPM to 0. Progress() should return 0 (guard at line 97).
	s.SetBPM(0)
	if got := s.Progress(); got != 0 {
		t.Fatalf("progress with BPM=0: expected 0, got %v", got)
	}
}

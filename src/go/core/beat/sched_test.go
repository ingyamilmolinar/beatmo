package beat

import (
	"math"
	"reflect"
	"testing"
	"time"
)

func TestSchedulerCatchUp(t *testing.T) {
	s := NewScheduler()
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
	s := NewScheduler()
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

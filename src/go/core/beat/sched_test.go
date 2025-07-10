package beat

import (
	"log"
	"testing"
	"time"
)

func TestSchedulerFiresEveryBeat(t *testing.T) {
	now := time.Now()
	fake := func() time.Time { now = now.Add(time.Second); return now }

	firedSteps := []int{}
	s := NewScheduler()
	s.BPM = 60
	s.now = fake
	s.OnTick = func(step int) {
		log.Printf("[TEST] OnTick called for step %d", step)
		firedSteps = append(firedSteps, step)
	}
	s.Start()

	for i := 0; i < s.BeatLength; i++ { // Loop BeatLength times to get all beats (0-15)
		s.Tick()
	}

	expectedSteps := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
	if len(firedSteps) != len(expectedSteps) {
		t.Fatalf("expected %d beats, got %d", len(expectedSteps), len(firedSteps))
	}
	for i, step := range firedSteps {
		if step != expectedSteps[i] {
			t.Fatalf("expected step %d, got %d at index %d", expectedSteps[i], step, i)
		}
	}
}

func TestFirstTickPlaysImmediately(t *testing.T) {
	now := time.Now()
	fake := func() time.Time { return now }

	fired := 0
	s := NewScheduler()
	s.BPM = 120
	s.now = fake
	s.OnTick = func(_ int) {
		log.Printf("[TEST] OnTick called from TestFirstTickPlaysImmediately")
		fired++
	}
	s.Start()

	if fired != 1 {
		t.Fatalf("expected first tick to fire once, got %d", fired)
	}
}

func TestSchedulerSkipsWhenBPMZero(t *testing.T) {
	now := time.Now()
	fake := func() time.Time { now = now.Add(time.Second); return now }

	fired := 0
	s := NewScheduler()
	s.BPM = 0
	s.now = fake
	s.OnTick = func(_ int) {
		log.Printf("[TEST] OnTick called from TestSchedulerSkipsWhenBPMZero")
		fired++
	}
	s.Start()
	s.Tick()
	if fired != 0 {
		t.Fatalf("expected no beats when BPM=0, got %d", fired)
	}
}
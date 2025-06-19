package beat

import (
	"testing"
	"time"

	"github.com/ingyamilmolinar/tunkul/core/model"
)

func TestSchedulerFiresEveryBeat(t *testing.T) {
	m := model.NewGraph()
	m.ToggleStep(0)

	now := time.Now()
	fake := func() time.Time { now = now.Add(time.Second); return now }

	fired := 0
	s := NewScheduler(m)
	s.BPM = 60
	s.now = fake
	s.OnBeat = func(_ int) { fired++ }

	for i := 0; i < 5; i++ {
		s.Tick()
	}
	if fired != 5 {
		t.Fatalf("expected 5 beats, got %d", fired)
	}
}

func TestFirstTickPlaysImmediately(t *testing.T) {
	m := model.NewGraph()
	m.ToggleStep(0)

	now := time.Now()
	fake := func() time.Time { return now }

	fired := 0
	s := NewScheduler(m)
	s.BPM = 120
	s.now = fake
	s.OnBeat = func(_ int) { fired++ }
	s.Tick()
	if fired != 1 {
		t.Fatalf("expected first tick to fire once, got %d", fired)
	}
}

func TestSchedulerSkipsWhenBPMZero(t *testing.T) {
	m := model.NewGraph()
	m.ToggleStep(0)
	now := time.Now()
	fake := func() time.Time { now = now.Add(time.Second); return now }

	fired := 0
	s := NewScheduler(m)
	s.BPM = 0
	s.now = fake
	s.OnBeat = func(_ int) { fired++ }
	s.Tick()
	if fired != 0 {
		t.Fatalf("expected no beats when BPM=0, got %d", fired)
	}
}

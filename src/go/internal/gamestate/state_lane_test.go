package gamestate

import (
	"testing"
	"time"
)

// StepAtAudioTime is the inverse of AudioTimeOfStep: the audio lane maps its
// horizon [now, now+H] on the audio clock to the abs steps it must commit.
func TestStepAtAudioTimeInvertsAudioTimeOfStep(t *testing.T) {
	var s State
	s.StartClock(time.Now(), 10.0)
	for idx := 0; idx < 64; idx++ {
		at := s.AudioTimeOfStep(idx, 4, 120)
		got, ok := s.StepAtAudioTime(at+1e-9, 120, 4)
		if !ok || got != idx {
			t.Fatalf("idx %d: StepAtAudioTime(%.6f)=%d ok=%v", idx, at, got, ok)
		}
		if mid, _ := s.StepAtAudioTime(at+0.05, 120, 4); mid != idx {
			t.Fatalf("idx %d: a time inside the step maps to %d", idx, mid)
		}
	}
	var none State
	if _, ok := none.StepAtAudioTime(1, 120, 4); ok {
		t.Fatal("no audio anchor must report ok=false")
	}
}

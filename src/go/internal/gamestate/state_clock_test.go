package gamestate

import (
	"math"
	"testing"
	"time"
)

func TestPosBeatsPrefersAudioClock(t *testing.T) {
	s := New(120)
	s.SetPlaying(true)
	s.SetBeatBaseForTest(4)    // start at beat 4
	s.SetAudioStartForTest(10) // audio clock anchored at t=10s
	// 120 BPM => 2 beats/sec. 3 audio-seconds later => +6 beats => 10.
	beats, ok := s.PosBeats(time.Now(), 13, 120)
	if !ok {
		t.Fatal("PosBeats ok=false with audio clock available")
	}
	if math.Abs(beats-10) > 1e-9 {
		t.Fatalf("PosBeats=%v want 10", beats)
	}
}

func TestPosBeatsFallsBackToWallClockWhenNoAudio(t *testing.T) {
	s := New(60)
	s.SetPlaying(true)
	s.SetBeatBaseForTest(0)
	// No audio clock (audioStart==0): must use wall clock.
	start := time.Now().Add(-2 * time.Second)
	s.SetPlayStart(start)
	beats, ok := s.PosBeats(time.Now(), 0, 60) // 60 BPM => 1 beat/sec => ~2
	if !ok {
		t.Fatal("PosBeats ok=false with wall clock available")
	}
	if beats < 1.5 || beats > 2.5 {
		t.Fatalf("PosBeats=%v want ~2", beats)
	}
}

func TestPosBeatsNotOkWithNoClock(t *testing.T) {
	s := New(120)
	s.SetPlaying(true)
	// playStart zero AND audioStart zero => no anchor.
	if _, ok := s.PosBeats(time.Now(), 0, 120); ok {
		t.Fatal("PosBeats ok=true with no clock anchor")
	}
}

func TestTargetStepFloorPosStepRound(t *testing.T) {
	s := New(120)
	s.SetPlaying(true)
	s.SetBeatBaseForTest(0)
	s.SetAudioStartForTest(0.0)
	s.SetPlayStart(time.Now().Add(-1 * time.Second)) // wall clock, ~2 beats at 120
	// Force an exact position via beatBase to avoid clock jitter in the assertion:
	s.SetPlayStart(time.Time{})
	s.SetBeatBaseForTest(2.6)
	// No clock => ok=false, but PosStep/TargetStep still return the base position.
	step, _ := s.PosStep(time.Now(), 0, 120, 4) // 2.6*4=10.4 => round 10
	if step != 10 {
		t.Fatalf("PosStep=%d want 10", step)
	}
	tgt, _ := s.TargetStep(time.Now(), 0, 120, 4) // floor(10.4)=10
	if tgt != 10 {
		t.Fatalf("TargetStep=%d want 10", tgt)
	}
}

func TestDtOfStepIsInverseOfPosBeats(t *testing.T) {
	s := New(140)
	s.SetPlaying(true)
	s.SetBeatBaseForTest(3)
	s.SetAudioStartForTest(5)
	div, bpm, idx := 8, 140, 40 // beat = 40/8 = 5.0
	// AudioTimeOfStep(idx) is the audio time at which the playhead reaches idx.
	at := s.AudioTimeOfStep(idx, div, bpm)
	beats, ok := s.PosBeats(time.Now(), at, bpm)
	if !ok {
		t.Fatal("PosBeats ok=false")
	}
	if math.Abs(beats-float64(idx)/float64(div)) > 1e-9 {
		t.Fatalf("round-trip beats=%v want %v", beats, float64(idx)/float64(div))
	}
}

// TestClockSingleSource pins that the display path and the scheduler path
// derive the SAME position from the SAME inputs — the drift this refactor
// eliminates stays eliminated.
func TestClockSingleSource(t *testing.T) {
	s := New(120)
	s.SetPlaying(true)
	s.SetBeatBaseForTest(1.25)
	s.SetAudioStartForTest(2)
	div, bpm := 4, 120
	now, audioNow := time.Now(), 4.5
	beats, ok := s.PosBeats(now, audioNow, bpm)
	if !ok {
		t.Fatal("PosBeats ok=false")
	}
	// Display path: raw beats. Scheduler path: floor(beats*div). Both must
	// come from the identical PosBeats value.
	wantTarget := int(math.Floor(beats * float64(div)))
	gotTarget, _ := s.TargetStep(now, audioNow, bpm, div)
	if gotTarget != wantTarget {
		t.Fatalf("TargetStep=%d want %d (scheduler must equal floor(PosBeats*div))", gotTarget, wantTarget)
	}
}

func TestSeekToPlayingReanchorsClocks(t *testing.T) {
	s := New(120)
	s.SetPlaying(true)
	before := time.Now().Add(-time.Hour)
	s.SetPlayStart(before)
	s.SeekTo(16, 4, time.Now(), 7) // beat 4
	if math.Abs(s.BeatBase()-4) > 1e-9 {
		t.Fatalf("BeatBase=%v want 4", s.BeatBase())
	}
	if s.PlayStart().Equal(before) {
		t.Fatal("SeekTo(playing) must re-anchor playStart")
	}
	if s.AudioStart() != 7 {
		t.Fatalf("AudioStart=%v want 7", s.AudioStart())
	}
}

func TestSeekToStoppedSetsPausedBeats(t *testing.T) {
	s := New(120)
	s.SetPlaying(false)
	s.SeekTo(12, 4, time.Now(), 0)
	if s.PausedBeats() != 12 {
		t.Fatalf("PausedBeats=%d want 12", s.PausedBeats())
	}
	if math.Abs(s.BeatBase()-3) > 1e-9 {
		t.Fatalf("BeatBase=%v want 3", s.BeatBase())
	}
}

func TestSetPositionSetsBaseAndMirrors(t *testing.T) {
	s := New(120)
	s.SetPosition(12, 4)
	if math.Abs(s.BeatBase()-3) > 1e-9 {
		t.Fatalf("BeatBase=%v want 3", s.BeatBase())
	}
	if s.PausedBeats() != 12 {
		t.Fatalf("PausedBeats=%d want 12", s.PausedBeats())
	}
	if s.LastStep() != 12 {
		t.Fatalf("LastStep=%d want 12", s.LastStep())
	}
	if math.Abs(s.LastBeat()-3) > 1e-9 {
		t.Fatalf("LastBeat=%v want 3", s.LastBeat())
	}
	if math.Abs(s.LastDisplayBeat()-3) > 1e-9 {
		t.Fatalf("LastDisplayBeat=%v want 3", s.LastDisplayBeat())
	}
}

func TestStartClockAnchorsWithoutTouchingPosition(t *testing.T) {
	s := New(120)
	s.SetBeatBaseForTest(5)
	now := time.Now()
	s.StartClock(now, 42)
	if !s.PlayStart().Equal(now) {
		t.Fatalf("PlayStart=%v want %v", s.PlayStart(), now)
	}
	if s.AudioStart() != 42 {
		t.Fatalf("AudioStart=%v want 42", s.AudioStart())
	}
	if math.Abs(s.BeatBase()-5) > 1e-9 {
		t.Fatalf("BeatBase=%v want unchanged 5", s.BeatBase())
	}
}

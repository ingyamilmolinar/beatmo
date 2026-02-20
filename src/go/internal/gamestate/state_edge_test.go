package gamestate

import (
	"math"
	"testing"
	"time"
)

func TestAnchorForBPMChangeWallClockFallback(t *testing.T) {
	s := New(100)
	startTime := time.Now().Add(-2 * time.Second)
	s.SetPlaying(true)
	s.SetPlayStart(startTime)
	s.SetAudioStart(0) // no audio clock

	now := time.Now()
	s.AnchorForBPMChange(100, now, 0)

	// Wall-clock delta is ~2s. Expected beatBase = 2 * 100/60 = ~3.333
	got := s.BeatBase()
	want := now.Sub(startTime).Seconds() * 100.0 / 60.0
	if math.Abs(got-want) > 0.05 {
		t.Fatalf("beatBase = %f, want ~%f (wall-clock fallback)", got, want)
	}

	// playStart should be updated to `now`
	if s.PlayStart() != now {
		t.Fatalf("playStart not updated after anchor")
	}

	// audioStart should remain 0 since audioNow was 0
	if s.AudioStart() != 0 {
		t.Fatalf("audioStart should stay 0 when audioNow=0, got %f", s.AudioStart())
	}
}

func TestResumeWithZeroAudioClock(t *testing.T) {
	s := New(120)
	now := time.Now()
	kind := s.Resume(now, 0)

	if kind != ResumeKindFresh {
		t.Fatalf("expected ResumeKindFresh, got %v", kind)
	}
	if !s.Playing() {
		t.Fatalf("expected Playing() true after resume")
	}
	if s.AudioStart() != 0 {
		t.Fatalf("AudioStart should be 0 when audioNow=0, got %f", s.AudioStart())
	}
	if s.PlayStart() != now {
		t.Fatalf("PlayStart should be set to now")
	}
}

func TestMultiplePauseResumeCycles(t *testing.T) {
	s := New(120)

	// Cycle 1: fresh start -> pause
	now := time.Now()
	kind := s.Resume(now, 1.0)
	if kind != ResumeKindFresh {
		t.Fatalf("cycle 1: expected fresh start, got %v", kind)
	}
	if s.BeatBase() != 0 {
		t.Fatalf("cycle 1: beatBase should be 0 on fresh start, got %f", s.BeatBase())
	}

	s.Pause(PauseInput{Beats: 16, GridDiv: 8, DisplayDiv: 16})
	if s.Playing() || !s.Paused() {
		t.Fatalf("cycle 1 pause: expected playing=false paused=true")
	}
	if s.PausedBeats() != 16 {
		t.Fatalf("cycle 1 pause: pausedBeats = %d want 16", s.PausedBeats())
	}
	// beatBase = 16/8 = 2.0
	if s.BeatBase() != 2.0 {
		t.Fatalf("cycle 1 pause: beatBase = %f want 2.0", s.BeatBase())
	}

	// Cycle 2: resume from pause -> pause again
	now2 := time.Now()
	kind = s.Resume(now2, 2.0)
	if kind != ResumeKindResume {
		t.Fatalf("cycle 2: expected ResumeKindResume, got %v", kind)
	}
	// beatBase should be lastBeat from pause, which is 2.0
	if s.BeatBase() != 2.0 {
		t.Fatalf("cycle 2 resume: beatBase = %f want 2.0", s.BeatBase())
	}
	if s.AudioStart() != 2.0 {
		t.Fatalf("cycle 2 resume: audioStart = %f want 2.0", s.AudioStart())
	}

	s.Pause(PauseInput{Beats: 32, GridDiv: 8, DisplayDiv: 16})
	// beatBase = 32/8 = 4.0
	if s.BeatBase() != 4.0 {
		t.Fatalf("cycle 2 pause: beatBase = %f want 4.0", s.BeatBase())
	}
	if s.PausedBeats() != 32 {
		t.Fatalf("cycle 2 pause: pausedBeats = %d want 32", s.PausedBeats())
	}

	// Cycle 3: resume again
	now3 := time.Now()
	kind = s.Resume(now3, 3.0)
	if kind != ResumeKindResume {
		t.Fatalf("cycle 3: expected ResumeKindResume, got %v", kind)
	}
	// beatBase should be lastBeat from second pause = 4.0
	if s.BeatBase() != 4.0 {
		t.Fatalf("cycle 3 resume: beatBase = %f want 4.0", s.BeatBase())
	}
	if !s.Playing() || s.Paused() {
		t.Fatalf("cycle 3: expected playing=true paused=false")
	}
}

func TestSnapshotDuringPause(t *testing.T) {
	s := New(90)
	s.Resume(time.Now(), 1.0)
	s.Pause(PauseInput{Beats: 24, GridDiv: 8, DisplayDiv: 16})

	snap := s.Snapshot()
	if snap.Playing {
		t.Fatalf("snapshot during pause: Playing should be false")
	}
	if !snap.Paused {
		t.Fatalf("snapshot during pause: Paused should be true")
	}
	if !snap.JustPaused {
		t.Fatalf("snapshot during pause: JustPaused should be true")
	}
	if snap.JustResumed {
		t.Fatalf("snapshot during pause: JustResumed should be false")
	}
	if snap.PausedBeats != 24 {
		t.Fatalf("snapshot during pause: PausedBeats = %d want 24", snap.PausedBeats)
	}
	if snap.BeatBase != 3.0 { // 24/8
		t.Fatalf("snapshot during pause: BeatBase = %f want 3.0", snap.BeatBase)
	}
	if snap.LastDisplayBeat != 1.5 { // 24/16
		t.Fatalf("snapshot during pause: LastDisplayBeat = %f want 1.5", snap.LastDisplayBeat)
	}
	if snap.AppliedBPM != 90 {
		t.Fatalf("snapshot during pause: AppliedBPM = %d want 90", snap.AppliedBPM)
	}
}

func TestSnapshotDuringPlay(t *testing.T) {
	s := New(140)
	s.Resume(time.Now(), 5.0)

	snap := s.Snapshot()
	if !snap.Playing {
		t.Fatalf("snapshot during play: Playing should be true")
	}
	if snap.Paused {
		t.Fatalf("snapshot during play: Paused should be false")
	}
	if snap.JustPaused {
		t.Fatalf("snapshot during play: JustPaused should be false")
	}
	if snap.BeatBase != 0 {
		t.Fatalf("snapshot during play: BeatBase = %f want 0 (fresh start)", snap.BeatBase)
	}
	if snap.AppliedBPM != 140 {
		t.Fatalf("snapshot during play: AppliedBPM = %d want 140", snap.AppliedBPM)
	}
}

func TestAnchorForBPMChangeUpdatesPlayStart(t *testing.T) {
	s := New(120)
	origStart := time.Now().Add(-3 * time.Second)
	s.Resume(origStart, 10.0)

	newNow := time.Now()
	s.AnchorForBPMChange(120, newNow, 12.0)

	if s.PlayStart() != newNow {
		t.Fatalf("AnchorForBPMChange should update playStart to now arg")
	}
	if s.AudioStart() != 12.0 {
		t.Fatalf("AnchorForBPMChange should update audioStart to audioNow when > 0, got %f", s.AudioStart())
	}
}

func TestFreshResumeResetsBeats(t *testing.T) {
	s := New(100)

	// Set up some state first via play -> pause -> stop
	s.Resume(time.Now(), 1.0)
	s.SetLastBeat(5.5)
	s.SetLastStep(44)
	s.SetBeatBase(5.5)
	s.Stop()

	// Fresh resume after stop
	now := time.Now()
	kind := s.Resume(now, 0)

	if kind != ResumeKindFresh {
		t.Fatalf("expected ResumeKindFresh after stop, got %v", kind)
	}
	if s.BeatBase() != 0 {
		t.Fatalf("fresh resume: BeatBase = %f want 0", s.BeatBase())
	}
	if s.LastBeat() != 0 {
		t.Fatalf("fresh resume: LastBeat = %f want 0", s.LastBeat())
	}
	if s.LastDisplayBeat() != 0 {
		t.Fatalf("fresh resume: LastDisplayBeat = %f want 0", s.LastDisplayBeat())
	}
	if s.LastStep() != 0 {
		t.Fatalf("fresh resume: LastStep = %d want 0", s.LastStep())
	}
	if !s.JustResumed() {
		t.Fatalf("fresh resume should set JustResumed")
	}
}

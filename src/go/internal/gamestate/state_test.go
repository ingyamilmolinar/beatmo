package gamestate

import (
	"testing"
	"time"
)

func TestPauseResumeAndSnapshot(t *testing.T) {
	state := New(140)

	state.Resume(time.Now(), 0) // establish playing state
	now := time.Now()
	state.Pause(PauseInput{
		Beats:      64,
		GridDiv:    16,
		DisplayDiv: 32,
	})

	if state.Playing() {
		t.Fatalf("expected paused state to clear playing flag")
	}
	if !state.Paused() {
		t.Fatalf("expected paused flag")
	}
	if got := state.PausedBeats(); got != 64 {
		t.Fatalf("paused beats = %d want 64", got)
	}
	if !state.JustPaused() {
		t.Fatalf("JustPaused should be true right after Pause")
	}

	state.ClearJustPaused()
	if state.JustPaused() {
		t.Fatalf("ClearJustPaused should reset flag")
	}

	audioStart := 10.0
	kind := state.Resume(now, audioStart)
	if kind != ResumeKindResume {
		t.Fatalf("expected resume kind Resume after pausing, got %v", kind)
	}
	if !state.Playing() || state.Paused() {
		t.Fatalf("expected playing=true, paused=false after resume")
	}
	if state.AudioStart() != audioStart {
		t.Fatalf("audio start mismatch got %f want %f", state.AudioStart(), audioStart)
	}
	if state.JustResumed() {
		t.Fatalf("resume from pause should not mark JustResumed")
	}
	state.ClearJustResumed()

	snap := state.Snapshot()
	if !snap.Playing || snap.Paused {
		t.Fatalf("snapshot inconsistent with live state: %+v", snap)
	}
	if snap.AppliedBPM != state.AppliedBPM() {
		t.Fatalf("snapshot bpm=%d want %d", snap.AppliedBPM, state.AppliedBPM())
	}
	if snap.PausedBeats != state.PausedBeats() {
		t.Fatalf("snapshot paused beats %d want %d", snap.PausedBeats, state.PausedBeats())
	}
}

func TestPauseInputClampsDivisors(t *testing.T) {
	state := New(120)
	state.Pause(PauseInput{Beats: 8}) // Zero divisors should clamp to 1
	if got := state.BeatBase(); got != 8 {
		t.Fatalf("beat base = %f want 8", got)
	}
	if got := state.LastDisplayBeat(); got != 8 {
		t.Fatalf("display beat = %f want 8", got)
	}
}

func TestStopResetsState(t *testing.T) {
	state := New(120)
	state.Resume(time.Now(), 5)
	state.SetSeekFreezeFrames(4)
	state.SetLastBeat(3.5)
	state.SetPausedBeats(12)
	state.Stop()

	if state.Playing() || state.Paused() {
		t.Fatalf("stop should clear playing+paused")
	}
	if state.BeatBase() != 0 || state.LastBeat() != 0 || state.LastDisplayBeat() != 0 {
		t.Fatalf("stop should reset beat counters")
	}
	if state.SeekFreezeFrames() != 0 {
		t.Fatalf("stop should clear seek freeze frames")
	}
	if state.PausedBeats() != 0 {
		t.Fatalf("stop should clear paused beats")
	}
}

func TestAnchorForBPMChangeUsesAudioClock(t *testing.T) {
	state := New(100)
	startAudio := 10.0
	state.Resume(time.Now(), startAudio)

	prevBase := state.BeatBase()
	state.AnchorForBPMChange(100, time.Now().Add(2*time.Second), startAudio+2)

	want := prevBase + (2 * 100 / 60.0)
	if got := state.BeatBase(); got != want {
		t.Fatalf("beat base after anchor = %f want %f", got, want)
	}
	if state.AudioStart() != startAudio+2 {
		t.Fatalf("audio start should advance with anchor, got %f", state.AudioStart())
	}
}

func TestSeekFreezeFramesCounter(t *testing.T) {
	state := New(120)
	state.SetSeekFreezeFrames(3)
	for i := 0; i < 2; i++ {
		state.DecSeekFreezeFrames()
	}
	if state.SeekFreezeFrames() != 1 {
		t.Fatalf("seek freeze frames expected 1 got %d", state.SeekFreezeFrames())
	}
	state.DecSeekFreezeFrames()
	state.DecSeekFreezeFrames() // extra decrement should clamp at zero
	if state.SeekFreezeFrames() != 0 {
		t.Fatalf("seek freeze frames should clamp to zero got %d", state.SeekFreezeFrames())
	}
}

func TestResumeFreshSetsJustResumed(t *testing.T) {
	state := New(120)
	now := time.Unix(0, 0)
	kind := state.Resume(now, 0)
	if kind != ResumeKindFresh {
		t.Fatalf("expected ResumeKindFresh, got %v", kind)
	}
	if !state.Playing() || state.Paused() {
		t.Fatalf("expected playing=true paused=false after fresh resume")
	}
	if !state.JustResumed() {
		t.Fatalf("expected JustResumed on fresh start")
	}
	if state.AudioStart() != 0 {
		t.Fatalf("expected audio start 0 when audioNow<=0, got %f", state.AudioStart())
	}
	state.ClearJustResumed()
	if state.JustResumed() {
		t.Fatalf("ClearJustResumed should reset flag")
	}
}

func TestAnchorForBPMChangeNoOpWhenStopped(t *testing.T) {
	state := New(120)
	state.SetBeatBase(4)
	state.AnchorForBPMChange(120, time.Unix(0, 0), 0)
	if got := state.BeatBase(); got != 4 {
		t.Fatalf("expected beat base unchanged while stopped, got %f", got)
	}
	state.SetPlaying(true)
	state.AnchorForBPMChange(0, time.Unix(0, 0), 0)
	if got := state.BeatBase(); got != 4 {
		t.Fatalf("expected beat base unchanged with prevBPM<=0, got %f", got)
	}
}

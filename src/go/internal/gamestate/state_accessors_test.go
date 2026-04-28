package gamestate

import (
	"testing"
	"time"
)

// TestStateAccessorsRoundTrip locks every (setter, getter) pair on State
// into a contract: calling the setter writes the value, calling the getter
// returns it. This catches the class of bug where someone changes the
// backing field type or atomic pointer without updating both sides — the
// kind of error that would otherwise show up as "playback dies after a
// BPM change" three weeks later.
//
// The accessors here are *not* covered transitively from Game tests
// because either the calling site uses the alias (e.g. SetPlaying ↔
// SetPlayingForTest) or the field is only mutated by the engine, never
// from a test.
func TestStateAccessorsRoundTrip(t *testing.T) {
	s := New(120)

	t.Run("AppliedBPM", func(t *testing.T) {
		s.SetAppliedBPM(177)
		if got := s.AppliedBPM(); got != 177 {
			t.Errorf("AppliedBPM=%d want 177", got)
		}
		s.SetAppliedBPM(0)
		if got := s.AppliedBPM(); got != 0 {
			t.Errorf("AppliedBPM=%d want 0", got)
		}
	})

	t.Run("Paused", func(t *testing.T) {
		s.SetPaused(true)
		if !s.Paused() {
			t.Error("Paused() should be true after SetPaused(true)")
		}
		s.SetPaused(false)
		if s.Paused() {
			t.Error("Paused() should be false after SetPaused(false)")
		}
	})

	t.Run("PlayingForTest_aliasesSetPlaying", func(t *testing.T) {
		// SetPlayingForTest is documented as an alias for SetPlaying;
		// they must be observationally identical.
		s.SetPlayingForTest(true)
		if !s.Playing() {
			t.Error("SetPlayingForTest(true) did not flip Playing()")
		}
		s.SetPlaying(false)
		s.SetPlayingForTest(true)
		if !s.Playing() {
			t.Error("SetPlayingForTest after SetPlaying(false) failed")
		}
		s.SetPlayingForTest(false)
		if s.Playing() {
			t.Error("SetPlayingForTest(false) did not clear Playing()")
		}
	})

	t.Run("PausedForTest_aliasesSetPaused", func(t *testing.T) {
		s.SetPaused(false)
		s.SetPausedForTest(true)
		if !s.Paused() {
			t.Error("SetPausedForTest(true) did not flip Paused()")
		}
		s.SetPausedForTest(false)
		if s.Paused() {
			t.Error("SetPausedForTest(false) did not clear Paused()")
		}
	})

	t.Run("LastDisplayBeat", func(t *testing.T) {
		s.SetLastDisplayBeat(12.5)
		if got := s.LastDisplayBeat(); got != 12.5 {
			t.Errorf("LastDisplayBeat=%v want 12.5", got)
		}
	})

	t.Run("LastProg", func(t *testing.T) {
		s.SetLastProg(0.42)
		if got := s.LastProg(); got != 0.42 {
			t.Errorf("LastProg=%v want 0.42", got)
		}
		// Boundary values: 0 and 1 are valid.
		s.SetLastProg(0)
		if got := s.LastProg(); got != 0 {
			t.Errorf("LastProg=%v want 0", got)
		}
		s.SetLastProg(1)
		if got := s.LastProg(); got != 1 {
			t.Errorf("LastProg=%v want 1", got)
		}
	})
}

// TestSnapshotReflectsAllSetters is the integration-flavored counterpart
// to the round-trip test: drive every public setter, then verify that
// Snapshot() reports each value. This guards against the bug where
// Snapshot stops mirroring a field after a refactor.
func TestSnapshotReflectsAllSetters(t *testing.T) {
	s := New(100)

	s.SetAppliedBPM(160)
	s.SetPlayingForTest(true)
	s.SetPausedForTest(true) // unusual, but the snapshot must still reflect it
	s.SetBeatBase(3.5)
	s.SetLastBeat(7.0)
	s.SetLastDisplayBeat(7.5)
	s.SetLastStep(42)
	s.SetPausedBeats(8)
	s.SetSeekFreezeFrames(3)

	snap := s.Snapshot()
	if snap.AppliedBPM != 160 || !snap.Playing || !snap.Paused ||
		snap.BeatBase != 3.5 || snap.LastBeat != 7.0 ||
		snap.LastDisplayBeat != 7.5 || snap.LastStep != 42 ||
		snap.PausedBeats != 8 || snap.SeekFreezeFrames != 3 {
		t.Errorf("snapshot does not match setters: %+v", snap)
	}
}

// TestSeekFreezeFramesDecrementClampsToZero validates the small but
// important invariant that DecSeekFreezeFrames stops at 0 instead of
// going negative — the freeze counter is consulted as `> 0` elsewhere.
func TestSeekFreezeFramesDecrementClampsToZero(t *testing.T) {
	s := New(120)
	s.SetSeekFreezeFrames(2)
	s.DecSeekFreezeFrames()
	s.DecSeekFreezeFrames()
	s.DecSeekFreezeFrames() // one extra tick
	if got := s.SeekFreezeFrames(); got != 0 {
		t.Errorf("SeekFreezeFrames=%d after over-decrementing, want 0", got)
	}
}

// TestPlayStartTimePropagation locks the contract that SetPlayStart and
// SetAudioStart are the canonical write paths consulted by tests when
// faking transport. We don't exercise the engine here — just the
// accessor pair — but a regression in either direction has caused
// silent hangs in past audits.
func TestPlayStartTimePropagation(t *testing.T) {
	s := New(120)
	now := time.Now().Truncate(time.Millisecond)
	s.SetPlayStart(now)
	if got := s.PlayStart(); !got.Equal(now) {
		t.Errorf("PlayStart=%v want %v", got, now)
	}
	s.SetAudioStart(1.25)
	if got := s.AudioStart(); got != 1.25 {
		t.Errorf("AudioStart=%v want 1.25", got)
	}
}

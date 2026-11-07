package gamestate

import (
	"testing"
	"time"
)

// Every setter/getter pair round-trips, catching a backing field changed on one side only; these are not covered
// through Game tests.
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

// Guards Snapshot silently ceasing to mirror a field after a refactor.
func TestSnapshotReflectsAllSetters(t *testing.T) {
	s := New(100)

	s.SetAppliedBPM(160)
	s.SetPlayingForTest(true)
	s.SetPausedForTest(true) // unusual, but the snapshot must still reflect it
	s.SetBeatBaseForTest(3.5)
	s.SetLastBeat(7.0)
	s.SetLastDisplayBeat(7.5)
	s.SetLastStep(42)
	s.SetPausedBeatsForTest(8)
	s.SetSeekFreezeFrames(3)

	snap := s.Snapshot()
	if snap.AppliedBPM != 160 || !snap.Playing || !snap.Paused ||
		snap.BeatBase != 3.5 || snap.LastBeat != 7.0 ||
		snap.LastDisplayBeat != 7.5 || snap.LastStep != 42 ||
		snap.PausedBeats != 8 || snap.SeekFreezeFrames != 3 {
		t.Errorf("snapshot does not match setters: %+v", snap)
	}
}

// The freeze counter is read as > 0, so it must stop at 0.
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

// The canonical write paths tests use to fake transport.
func TestPlayStartTimePropagation(t *testing.T) {
	s := New(120)
	now := time.Now().Truncate(time.Millisecond)
	s.SetPlayStart(now)
	if got := s.PlayStart(); !got.Equal(now) {
		t.Errorf("PlayStart=%v want %v", got, now)
	}
	s.SetAudioStartForTest(1.25)
	if got := s.AudioStart(); got != 1.25 {
		t.Errorf("AudioStart=%v want 1.25", got)
	}
}

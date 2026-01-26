package gamestate

import (
	"sync/atomic"
	"time"
)

// ResumeKind describes the transition performed when toggling playback on.
type ResumeKind int

const (
	// ResumeKindFresh indicates playback started from the beginning (was not paused).
	ResumeKindFresh ResumeKind = iota
	// ResumeKindResume indicates playback resumed from a paused position.
	ResumeKindResume
)

// State tracks transport-related values (play/pause/start time, beat anchors, etc.)
// independent of rendering concerns.
type State struct {
	playing          atomic.Bool
	paused           atomic.Bool
	beatBase         float64
	lastBeat         float64
	lastDisplayBeat  float64
	lastStep         int
	playStart        time.Time
	audioStart       float64
	appliedBPM       atomic.Int32
	justPaused       atomic.Bool
	justResumed      atomic.Bool
	pausedBeats      int
	seekFreezeFrames int
	lastProg         float64
}

// PauseInput defines frame snapshot data required to enter a paused state.
type PauseInput struct {
	Beats      int
	GridDiv    int
	DisplayDiv int
}

// New creates a State with the provided initially applied BPM.
func New(initialBPM int) *State {
	s := &State{}
	s.appliedBPM.Store(int32(initialBPM))
	return s
}

// Pause transitions into a paused state using the provided frame snapshot.
func (s *State) Pause(in PauseInput) {
	if in.GridDiv <= 0 {
		in.GridDiv = 1
	}
	if in.DisplayDiv <= 0 {
		in.DisplayDiv = 1
	}
	s.playing.Store(false)
	s.paused.Store(true)
	s.pausedBeats = in.Beats
	s.beatBase = float64(in.Beats) / float64(in.GridDiv)
	s.lastBeat = s.beatBase
	s.lastDisplayBeat = float64(in.Beats) / float64(in.DisplayDiv)
	s.lastStep = in.Beats
	s.playStart = time.Time{}
	s.audioStart = 0
	s.justPaused.Store(true)
	s.justResumed.Store(false)
}

// Resume toggles playback on (either fresh start or from pause) and returns the transition kind.
func (s *State) Resume(now time.Time, audioNow float64) ResumeKind {
	wasPaused := s.paused.Load()
	if wasPaused {
		s.beatBase = s.lastBeat
		s.lastStep = s.pausedBeats
		s.justResumed.Store(false)
	} else {
		s.beatBase = 0
		s.lastBeat = 0
		s.lastDisplayBeat = 0
		s.lastStep = 0
		s.justResumed.Store(true)
	}
	s.playing.Store(true)
	s.paused.Store(false)
	s.justPaused.Store(false)
	s.playStart = now
	if audioNow > 0 {
		s.audioStart = audioNow
	} else {
		s.audioStart = 0
	}
	if wasPaused {
		return ResumeKindResume
	}
	return ResumeKindFresh
}

// Stop resets playback completely to an idle state.
func (s *State) Stop() {
	s.playing.Store(false)
	s.paused.Store(false)
	s.beatBase = 0
	s.lastBeat = 0
	s.lastDisplayBeat = 0
	s.lastStep = 0
	s.playStart = time.Time{}
	s.audioStart = 0
	s.justPaused.Store(false)
	s.justResumed.Store(false)
	s.pausedBeats = 0
	s.seekFreezeFrames = 0
}

// AnchorForBPMChange re-anchors the beat base to preserve continuity when BPM changes mid-playback.
func (s *State) AnchorForBPMChange(prevBPM int, now time.Time, audioNow float64) {
	if !s.playing.Load() || prevBPM <= 0 {
		return
	}
	var dtSec float64
	if audioNow > 0 && s.audioStart > 0 {
		dtSec = audioNow - s.audioStart
	} else if !s.playStart.IsZero() {
		dtSec = now.Sub(s.playStart).Seconds()
	}
	s.beatBase += dtSec * float64(prevBPM) / 60.0
	s.playStart = now
	if audioNow > 0 {
		s.audioStart = audioNow
	}
}

// SetAppliedBPM updates the applied BPM after the engine acknowledges a change.
func (s *State) SetAppliedBPM(bpm int) { s.appliedBPM.Store(int32(bpm)) }

// AppliedBPM returns the engine-applied BPM.
func (s *State) AppliedBPM() int { return int(s.appliedBPM.Load()) }

// Playing reports whether playback is currently active.
func (s *State) Playing() bool { return s.playing.Load() }

// Paused reports whether playback is paused (but not stopped).
func (s *State) Paused() bool { return s.paused.Load() }

// SetPlaying forces the playing flag without side effects (test helper).
func (s *State) SetPlaying(v bool) { s.playing.Store(v) }

// SetPaused forces the paused flag without side effects (test helper).
func (s *State) SetPaused(v bool) { s.paused.Store(v) }

// SetPlayingForTest mirrors SetPlaying while ensuring compatibility with legacy helpers.
func (s *State) SetPlayingForTest(v bool) { s.SetPlaying(v) }

// SetPausedForTest mirrors SetPaused for backwards compatibility with older tests.
func (s *State) SetPausedForTest(v bool) { s.SetPaused(v) }

// BeatBase returns the base beat offset used for time calculations.
func (s *State) BeatBase() float64 { return s.beatBase }

// SetBeatBase forces a new beat base (mainly for tests).
func (s *State) SetBeatBase(v float64) { s.beatBase = v }

// LastBeat returns the last quantized beat value.
func (s *State) LastBeat() float64 { return s.lastBeat }

// SetLastBeat updates the cached last beat.
func (s *State) SetLastBeat(v float64) { s.lastBeat = v }

// LastDisplayBeat returns the beat value used for UI counters.
func (s *State) LastDisplayBeat() float64 { return s.lastDisplayBeat }

// SetLastDisplayBeat updates the cached display beat.
func (s *State) SetLastDisplayBeat(v float64) { s.lastDisplayBeat = v }

// LastStep returns the last absolute subdivision index for the primary row.
func (s *State) LastStep() int { return s.lastStep }

// SetLastStep records the latest absolute subdivision index.
func (s *State) SetLastStep(v int) { s.lastStep = v }

// PlayStart returns the wall-clock start time of playback.
func (s *State) PlayStart() time.Time { return s.playStart }

// SetPlayStart updates the wall-clock start time.
func (s *State) SetPlayStart(t time.Time) { s.playStart = t }

// AudioStart returns the audio clock start position.
func (s *State) AudioStart() float64 { return s.audioStart }

// SetAudioStart updates the audio clock start position.
func (s *State) SetAudioStart(v float64) { s.audioStart = v }

// JustPaused reports whether Pause was triggered this frame.
func (s *State) JustPaused() bool { return s.justPaused.Load() }

// ClearJustPaused resets the one-frame pause guard.
func (s *State) ClearJustPaused() { s.justPaused.Store(false) }

// JustResumed reports whether a fresh start occurred this frame.
func (s *State) JustResumed() bool { return s.justResumed.Load() }

// ClearJustResumed resets the one-frame resume guard.
func (s *State) ClearJustResumed() { s.justResumed.Store(false) }

// PausedBeats returns the subdivision index where playback paused.
func (s *State) PausedBeats() int { return s.pausedBeats }

// SetPausedBeats updates the stored pause index.
func (s *State) SetPausedBeats(v int) { s.pausedBeats = v }

// SeekFreezeFrames returns the remaining seek freeze frames.
func (s *State) SeekFreezeFrames() int { return s.seekFreezeFrames }

// SetSeekFreezeFrames sets the freeze frame counter.
func (s *State) SetSeekFreezeFrames(v int) { s.seekFreezeFrames = v }

// DecSeekFreezeFrames decrements the freeze frame counter if positive.
func (s *State) DecSeekFreezeFrames() {
	if s.seekFreezeFrames > 0 {
		s.seekFreezeFrames--
	}
}

// LastProg returns the previous fractional progress (0..1) for beat smoothing.
func (s *State) LastProg() float64 { return s.lastProg }

// SetLastProg stores the fractional progress for smoothing.
func (s *State) SetLastProg(v float64) { s.lastProg = v }

// Snapshot captures the current transport state for diagnostics or external consumers.
type Snapshot struct {
	Playing          bool
	Paused           bool
	AppliedBPM       int
	BeatBase         float64
	LastBeat         float64
	LastDisplayBeat  float64
	LastStep         int
	PausedBeats      int
	SeekFreezeFrames int
	JustPaused       bool
	JustResumed      bool
}

// Snapshot returns a point-in-time copy of the State.
func (s *State) Snapshot() Snapshot {
	return Snapshot{
		Playing:          s.playing.Load(),
		Paused:           s.paused.Load(),
		AppliedBPM:       int(s.appliedBPM.Load()),
		BeatBase:         s.beatBase,
		LastBeat:         s.lastBeat,
		LastDisplayBeat:  s.lastDisplayBeat,
		LastStep:         s.lastStep,
		PausedBeats:      s.pausedBeats,
		SeekFreezeFrames: s.seekFreezeFrames,
		JustPaused:       s.justPaused.Load(),
		JustResumed:      s.justResumed.Load(),
	}
}

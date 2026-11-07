package gamestate

import (
	"math"
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
	dtSec, ok := s.DtSince(now, audioNow)
	if !ok {
		return
	}
	s.beatBase += dtSec * float64(prevBPM) / 60.0
	s.playStart = now
	if audioNow > 0 {
		s.audioStart = audioNow
	}
}

// The SINGLE place the audio-vs-wall clock choice lives: the audio clock when anchored, else wall. ok=false = no anchor.
func (s *State) DtSince(now time.Time, audioNow float64) (dtSec float64, ok bool) {
	if audioNow > 0 && s.audioStart > 0 {
		return audioNow - s.audioStart, true
	}
	if !s.playStart.IsZero() {
		return now.Sub(s.playStart).Seconds(), true
	}
	return 0, false
}

// PosBeats is the ONE transport-position formula: beatBase + elapsed*bpm/60.
// ok mirrors DtSince; when !ok it returns beatBase (no progress). Callers apply
// their own floor/round and monotonic clamping.
func (s *State) PosBeats(now time.Time, audioNow float64, bpm int) (beats float64, ok bool) {
	dt, ok := s.DtSince(now, audioNow)
	if !ok {
		return s.beatBase, false
	}
	return s.beatBase + dt*float64(bpm)/60.0, true
}

// PosStep is the canonical absolute-subdivision position (round).
func (s *State) PosStep(now time.Time, audioNow float64, bpm, div int) (int, bool) {
	if div <= 0 {
		div = 1
	}
	b, ok := s.PosBeats(now, audioNow, bpm)
	return int(math.Round(b * float64(div))), ok
}

// TargetStep is the scheduler's catch-up target (floor).
func (s *State) TargetStep(now time.Time, audioNow float64, bpm, div int) (int, bool) {
	if div <= 0 {
		div = 1
	}
	b, ok := s.PosBeats(now, audioNow, bpm)
	return int(math.Floor(b * float64(div))), ok
}

// DtOfStep is the inverse of PosBeats: elapsed seconds at which the playhead
// reaches subdivision idx, relative to the play anchor.
func (s *State) DtOfStep(idx, div, bpm int) float64 {
	if div <= 0 {
		div = 1
	}
	if bpm <= 0 {
		return 0
	}
	return (float64(idx)/float64(div) - s.beatBase) * 60.0 / float64(bpm)
}

// ok is false without an audio anchor: the audio lane schedules on the audio clock and has no wall fallback.
func (s *State) StepAtAudioTime(t float64, bpm, div int) (int, bool) {
	if s.audioStart <= 0 || bpm <= 0 {
		return 0, false
	}
	if div <= 0 {
		div = 1
	}
	beats := s.beatBase + (t-s.audioStart)*float64(bpm)/60.0
	return int(math.Floor(beats*float64(div) + 1e-9)), true
}

// AnchorKey exposes the clock anchor for change detection (the audio lane's
// fingerprint): every re-anchor — play, seek, BPM change — changes it.
func (s *State) AnchorKey() (beatBase, audioStart float64) { return s.beatBase, s.audioStart }

// AudioTimeOfStep is DtOfStep expressed on the audio clock — the ideal audio
// time to schedule subdivision idx.
func (s *State) AudioTimeOfStep(idx, div, bpm int) float64 {
	return s.audioStart + s.DtOfStep(idx, div, bpm)
}

// The single mutation that moves the transport: playing re-anchors so PosBeats continues from steps; stopped or paused
// records the paused position.
func (s *State) SeekTo(steps, div int, now time.Time, audioNow float64) {
	if div <= 0 {
		div = 1
	}
	beat := float64(steps) / float64(div)
	s.beatBase = beat
	s.lastBeat = beat
	s.lastDisplayBeat = beat
	s.lastProg = 0
	s.lastStep = steps
	if s.playing.Load() {
		s.playStart = now
		if audioNow > 0 {
			s.audioStart = audioNow
		} else {
			s.audioStart = 0
		}
		s.justResumed.Store(false)
	} else {
		s.pausedBeats = steps
	}
}

// Leaves the clocks alone (callers that re-anchor do it themselves); for the resume-from-offset and media-session paths.
func (s *State) SetPosition(steps, div int) {
	if div <= 0 {
		div = 1
	}
	beat := float64(steps) / float64(div)
	s.beatBase = beat
	s.lastBeat = beat
	s.lastDisplayBeat = beat
	s.lastStep = steps
	s.pausedBeats = steps
}

// StartClock anchors the wall/audio clock for playback started programmatically
// (not via the Play button), leaving the position fields untouched.
func (s *State) StartClock(now time.Time, audioNow float64) {
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

// SetBeatBaseForTest forces a new beat base (tests only — production code must
// go through SeekTo / the Resume/Pause/Stop transitions).
func (s *State) SetBeatBaseForTest(v float64) { s.beatBase = v }

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

// SetAudioStartForTest forces the audio clock anchor (tests only).
func (s *State) SetAudioStartForTest(v float64) { s.audioStart = v }

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

// SetPausedBeatsForTest forces the stored pause index (tests only).
func (s *State) SetPausedBeatsForTest(v int) { s.pausedBeats = v }

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

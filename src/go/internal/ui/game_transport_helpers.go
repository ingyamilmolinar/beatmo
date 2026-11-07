package ui

import (
	"time"
)

// Transport helpers (used by tests to introspect gamestate.State).
func (g *Game) Playing() bool { return g.state.Playing() }

// Enforced at the single PlayPressed() chokepoint, NOT in SetPlaying, which tests, scenes and
// benches drive directly.
func (g *Game) playbackReady() bool { return g.warmup == nil || g.warmup.Ready() }

func (g *Game) Paused() bool { return g.state.Paused() }

func (g *Game) AppliedBPM() int { return g.state.AppliedBPM() }

func (g *Game) BeatBase() float64 { return g.state.BeatBase() }

func (g *Game) LastBeat() float64 { return g.state.LastBeat() }

func (g *Game) LastDisplayBeat() float64 { return g.state.LastDisplayBeat() }

func (g *Game) LastProg() float64 { return g.state.LastProg() }

// Lets a test assert resume work is bounded by the predictor window, not the elapsed playhead.
func (g *Game) FreezeBackfillIters() int64 { return g.freezeBackfillIters }

func (g *Game) SetPlaying(v bool) {
	g.state.SetPlaying(v)
	if v {
		g.state.SetPaused(false)
		g.state.ClearJustPaused()
	} else {
		g.state.ClearJustResumed()
	}
	g.drum.SetPlaying(v)
}

func (g *Game) PlayStart() time.Time { return g.state.PlayStart() }

func (g *Game) AudioStart() float64 { return g.state.AudioStart() }

func (g *Game) PausedBeats() int { return g.state.PausedBeats() }

func (g *Game) SeekFreezeFrames() int { return g.state.SeekFreezeFrames() }

func (g *Game) JustResumed() bool { return g.state.JustResumed() }

func (g *Game) JustPaused() bool { return g.state.JustPaused() }

func (g *Game) SetPlayingForTest(v bool) { g.state.SetPlayingForTest(v) }

func (g *Game) SetPausedForTest(v bool) { g.state.SetPausedForTest(v) }

func (g *Game) SetAppliedBPMForTest(bpm int) { g.state.SetAppliedBPM(bpm) }

func (g *Game) SetBeatBaseForTest(v float64) { g.state.SetBeatBaseForTest(v) }

func (g *Game) SetLastBeatForTest(v float64) { g.state.SetLastBeat(v) }

func (g *Game) SetLastDisplayBeatForTest(v float64) { g.state.SetLastDisplayBeat(v) }

func (g *Game) SetPlayStartForTest(t time.Time) { g.state.SetPlayStart(t) }

func (g *Game) SetAudioStartForTest(v float64) { g.state.SetAudioStartForTest(v) }

func (g *Game) SetPausedBeatsForTest(v int) { g.state.SetPausedBeatsForTest(v) }

func (g *Game) SetSeekFreezeFrames(v int) { g.state.SetSeekFreezeFrames(v) }

func (g *Game) SetRowSnapshotModeForTest(enabled bool) { g.rowSnapshotMode = enabled }

func (g *Game) StopBackgroundPredictorForTest() {
	if g == nil || g.engine == nil || g.engine.Predictor == nil {
		return
	}
	if g.predictorBackgroundStopped {
		return
	}
	g.engine.Predictor.StopBackground()
	g.predictorBackgroundStopped = true
}

func (g *Game) StopSequencerForTest() {
	if g == nil || g.seqQuit == nil {
		return
	}
	if g.sequencerStopped {
		return
	}
	close(g.seqQuit)
	g.sequencerRunning = false
	g.sequencerStopped = true
}

func (g *Game) ClearJustResumed() { g.state.ClearJustResumed() }

func (g *Game) ClearJustPaused() { g.state.ClearJustPaused() }

// Idempotent.
func (g *Game) CloseForTest() {
	g.Close()
}

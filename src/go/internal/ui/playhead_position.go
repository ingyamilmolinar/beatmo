package ui

import "math"

// PlayheadPos is the canonical, display-ready playhead position: ONE sample of
// the transport clock resolved into every quantity a readout needs.
//
// Every surface that answers "where is the playhead?" — the transport beat
// counter (timeline_zone.go) and the navigation menu's position title
// (navigation_panel.go) — renders from this struct. Surfaces must NOT re-derive
// beat/second values from raw clock quantities on their own: two independent
// derivations are exactly how those two readouts drifted apart. The counter
// floored the continuous beat and measured continuous elapsed time, while the
// nav menu floored the *rounded subdivision* and measured the whole-beat
// boundary time — so at 3.9 beats / 120 BPM the counter read "Beat 4 · 0:01"
// while the nav menu read "Beat 3 · 0:02", disagreeing on both fields.
//
// Beat numbering: Beats and Steps are 0-based — the units the scheduler, the
// seek API, the grid axis labels and the go-to-beat editor all speak.
// DisplayBeat is the 1-based number the user reads; internal beat 0 is
// "Beat 1" on screen.
type PlayheadPos struct {
	Beats       float64 // continuous 0-based beat position (the clock itself)
	Steps       int     // absolute 0-based subdivision index (the scheduler's unit)
	DisplayBeat int     // 1-based whole beat, for user-facing readouts only
	Seconds     int     // whole elapsed seconds
}

// playheadPosAt resolves one clock sample (continuous beats + the tempo and
// subdivision in force) into every readout quantity. The single derivation
// shared by every position readout — change the rounding here, not at a call
// site.
func playheadPosAt(beats, secPerBeat float64, div int) PlayheadPos {
	if div <= 0 {
		div = 1
	}
	if beats < 0 {
		beats = 0
	}
	if secPerBeat <= 0 {
		secPerBeat = 60.0 / 120.0 // 120 BPM, matching the app's default tempo
	}
	return PlayheadPos{
		Beats:       beats,
		Steps:       int(math.Round(beats * float64(div))),
		DisplayBeat: int(beats) + 1,
		// Elapsed wall time of the continuous position, floored to whole
		// seconds (the millisecond round mirrors the counter's cache key).
		Seconds: int(math.Round(beats*secPerBeat*1000.0)) / 1000,
	}
}

// PlayheadPosition is the transport/scheduler API every position readout reads.
// It samples the same clock the sequencer and the seek path use (displayBeat,
// which already fuses tick progress with smooth wall/audio-clock interpolation)
// and resolves it once via playheadPosAt.
func (g *Game) PlayheadPosition() PlayheadPos {
	bpm := g.AppliedBPM()
	if bpm <= 0 {
		bpm = g.bpm
	}
	secPerBeat := 0.0
	if bpm > 0 {
		secPerBeat = 60.0 / float64(bpm)
	}
	return playheadPosAt(g.displayBeat(), secPerBeat, max1(g.grid.MaxDiv()))
}

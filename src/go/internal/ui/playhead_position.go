package ui

import "math"

// ONE clock sample resolved into every readout quantity: two independent derivations are how the beat counter and
// the nav title drifted apart. Beats/Steps are 0-based; DisplayBeat is the 1-based number the user reads.
type PlayheadPos struct {
	Beats       float64 // continuous 0-based beat position (the clock itself)
	Steps       int     // absolute 0-based subdivision index (the scheduler's unit)
	DisplayBeat int     // 1-based whole beat, for user-facing readouts only
	Seconds     int     // whole elapsed seconds
}

// Change the rounding here, never at a call site.
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
		// Floored to seconds; the millisecond round mirrors the counter's cache key.
		Seconds: int(math.Round(beats*secPerBeat*1000.0)) / 1000,
	}
}

// The same clock the sequencer and seek use, resolved once.
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

package ui

import "github.com/ingyamilmolinar/beatmo/internal/audio"

// beatGridFractions returns fractional X positions in [0,1) for each
// subdivision tick that falls inside a wave window of windowSamples
// duration (samples at audio.SampleRate()), starting at the current
// playhead. Walks forward from g.playheadFloor() while sampleOffset is
// inside the window.
//
// The wave panel renders a ~46 ms snapshot (~2200 samples @ 48 kHz). At
// BPM=120 / subdiv=8 a single tick spans ~62.5 ms (~3000 samples), so
// the dense default window catches at most one tick. Wider windows (the
// zoomed-out wave) emit more.
//
// User decision: ticks are placed at subdivision boundaries — not literal
// beats — because beats (~500 ms @ 120 BPM) would never fit in the wave
// window.
func (g *Game) beatGridFractions(rowIdx int, windowSamples int) []float64 {
	if g == nil || rowIdx < 0 {
		return nil
	}
	if rowIdx >= len(g.beatInfosByRow) {
		return nil
	}
	if windowSamples <= 0 {
		return nil
	}
	bpm := g.drum.BPM()
	if bpm <= 0 {
		return nil
	}
	subdiv := g.grid.MaxDiv()
	if subdiv <= 0 {
		return nil
	}
	sr := audio.SampleRate()
	if sr <= 0 {
		return nil
	}
	// Samples per subdivision tick at the current BPM and subdivision count.
	samplesPerTick := float64(sr) * 60.0 / (float64(bpm) * float64(subdiv))
	if samplesPerTick <= 0 {
		return nil
	}
	out := make([]float64, 0, 16)
	// Walk forward from the current playhead. The beatInfoAtRow lookup is
	// kept here (even though its result is unused below) so future logic
	// gating on mute/invisible cells can hook in without changing call
	// sites; for now every subdivision slot emits a tick so the visual
	// grid stays evenly spaced.
	startIdx := g.playheadFloor()
	for i := 0; ; i++ {
		sampleOffset := float64(i) * samplesPerTick
		if sampleOffset >= float64(windowSamples) {
			break
		}
		_ = g.beatInfoAtRow(rowIdx, startIdx+i)
		frac := sampleOffset / float64(windowSamples)
		if frac >= 0 && frac < 1 {
			out = append(out, frac)
		}
	}
	return out
}

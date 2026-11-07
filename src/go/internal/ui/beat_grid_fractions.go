package ui

import "github.com/ingyamilmolinar/beatmo/internal/audio"

// Ticks sit on SUBDIVISION boundaries, not beats (user decision): a ~500 ms beat never fits the ~46 ms wave window.
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
	// Every subdivision emits a tick so the grid stays evenly spaced; the beatInfoAtRow result is unused.
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

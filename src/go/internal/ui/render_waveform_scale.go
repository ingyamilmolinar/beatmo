package ui

import (
	"fmt"

	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
)

// Wave-tab adaptive Y-scale constants. Mirrors the Chain/Scope auto-gain model
// (chain_trace_cache.go:98-111): target gain fills ~90% of the panel at the
// signal's peak, clamped so a quiet signal scales up but never past 16x and a
// hot signal never shrinks below 1x.
const (
	waveAutoGainTargetFill = 0.9   // peak maps to 90% of half-height
	waveAutoGainMax        = 16.0  // clamp ceiling (matches chain)
	waveAutoGainMin        = 1.0   // never zoom out under auto (matches chain)
	waveAutoGainSilence    = 0.001 // below this peak, treat as silence -> 1x
	// waveAutoGainRelease is the per-data-refresh decay factor for the smoothed
	// peak-hold. The analyzer republishes ~30 Hz, so 0.90 gives a ~150 ms scale
	// decay (instant attack on a louder hit). Tunable; keep the rationale.
	waveAutoGainRelease = 0.90
)

// autoGainForPeak maps a (smoothed) peak amplitude to an effective Y-gain.
// Pure; unit-tested. Silence -> 1.0; otherwise clamp(0.9/peak, 1.0, 16.0).
func autoGainForPeak(peak float64) float64 {
	if peak < waveAutoGainSilence {
		return waveAutoGainMin
	}
	g := waveAutoGainTargetFill / peak
	if g < waveAutoGainMin {
		g = waveAutoGainMin
	}
	if g > waveAutoGainMax {
		g = waveAutoGainMax
	}
	return g
}

// advanceWavePeak applies smoothed peak-hold: instant attack (a louder peak
// wins immediately), slow release (a quieter peak decays the held value toward
// it by waveAutoGainRelease). Pure; unit-tested.
func advanceWavePeak(prev, peak float64) float64 {
	if peak >= prev {
		return peak
	}
	return prev * waveAutoGainRelease
}

// waveEdgeAmplitude returns the true sample amplitude represented by the top
// (and, negated, the bottom) edge of the wave panel at the given gain. At gain
// g the panel edge corresponds to amplitude 1/g.
func waveEdgeAmplitude(gain float64) float64 {
	if gain <= 0 {
		return 1.0
	}
	return 1.0 / gain
}

// waveGainBadgeText formats the corner Y-scale badge. "AG x4.2" when auto-
// gain is engaged, "x2.0" when manually zoomed. "AG" is the one label for
// the auto-gain concept everywhere (Chain pill, Wave pill + badge); the "x"
// is a plain letter (not the forbidden multiplication glyph).
func waveGainBadgeText(gain float64, autoOn bool) string {
	if autoOn {
		return fmt.Sprintf("AG x%.1f", gain)
	}
	return fmt.Sprintf("x%.1f", gain)
}

// waveWindowPeak returns the max |sample| over the waveform the Wave tab is
// currently showing — frozen capture wins, else the rolling channel waveform,
// scanning stereo L/R when present. Used to drive the adaptive gain.
func waveWindowPeak(ch *analyzer.ChannelMetrics, capture *analyzer.CaptureBuffer) float64 {
	var wave []float64
	switch {
	case capture != nil && capture.Frozen:
		wave = capture.Wave.Samples
	case ch != nil && ch.Active:
		wave = ch.Waveform
	}
	peak := maxAbs(wave)
	if ch != nil && ch.HasStereo() {
		if l := maxAbs(ch.WaveformL); l > peak {
			peak = l
		}
		if r := maxAbs(ch.WaveformR); r > peak {
			peak = r
		}
	}
	return peak
}

func maxAbs(xs []float64) float64 {
	m := 0.0
	for _, v := range xs {
		if v < 0 {
			v = -v
		}
		if v > m {
			m = v
		}
	}
	return m
}

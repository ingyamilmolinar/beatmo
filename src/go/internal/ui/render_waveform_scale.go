package ui

import (
	"fmt"

	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
)

// The Chain auto-gain model: the peak fills ~90% of the panel, clamped to 1x..16x.
const (
	waveAutoGainTargetFill = 0.9   // peak maps to 90% of half-height
	waveAutoGainMax        = 16.0  // clamp ceiling (matches chain)
	waveAutoGainMin        = 1.0   // never zoom out under auto (matches chain)
	waveAutoGainSilence    = 0.001 // below this peak, treat as silence -> 1x
	// Per analyzer refresh (~30 Hz), so ~150 ms release; attack is instant.
	waveAutoGainRelease = 0.90
)

// Silence → 1.0; otherwise clamp(0.9/peak, 1, 16).
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

func advanceWavePeak(prev, peak float64) float64 {
	if peak >= prev {
		return peak
	}
	return prev * waveAutoGainRelease
}

// At gain g the panel edge is amplitude 1/g.
func waveEdgeAmplitude(gain float64) float64 {
	if gain <= 0 {
		return 1.0
	}
	return 1.0 / gain
}

// "AG" is the one auto-gain label everywhere; "x" is a plain letter, not the forbidden glyph.
func waveGainBadgeText(gain float64, autoOn bool) string {
	if autoOn {
		return fmt.Sprintf("AG x%.1f", gain)
	}
	return fmt.Sprintf("x%.1f", gain)
}

// Frozen capture wins; stereo scans L and R.
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

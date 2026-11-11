package ui

import (
	"fmt"
	"math"
)

// dBFromLinear converts a linear amplitude to dB. Negative or zero
// amplitudes return -Inf.
func dBFromLinear(v float64) float64 {
	if v <= 0 {
		return -math.Inf(1)
	}
	return 20 * math.Log10(v)
}

// clampDBDisplay clamps -Inf dB to -96 for display.
func clampDBDisplay(db float64) float64 {
	if math.IsInf(db, -1) {
		return -96.0
	}
	return db
}

// formatWindowMs formats a millisecond duration for scope axis labels.
// >=10ms uses 0 decimals, >=1ms uses 1 decimal, sub-ms uses 2 decimals.
func formatWindowMs(ms float64) string {
	if ms >= 10 {
		return fmt.Sprintf("%.0fms", ms)
	}
	if ms >= 1 {
		return fmt.Sprintf("%.1fms", ms)
	}
	return fmt.Sprintf("%.2fms", ms)
}

// WaveAxisLabel describes one tick position along the Wave-tab time axis.
// Frac is the position as a [0,1] fraction of the wave width; Text is the
// formatted label (e.g. "0.00ms", "5.0ms", "20ms").
type WaveAxisLabel struct {
	Frac float64
	Text string
}

// waveAxisLabels returns the five canonical tick positions for the Wave
// tab time axis: 0%, 25%, 50%, 75%, 100% of the current window length.
// The leftmost label is always rendered as "0.00ms" to anchor the axis
// origin distinctly from other ticks (mirrors Chain's convention).
func waveAxisLabels(windowMs float64) []WaveAxisLabel {
	out := make([]WaveAxisLabel, 5)
	for i := 0; i < 5; i++ {
		f := float64(i) / 4.0
		t := f * windowMs
		txt := formatWindowMs(t)
		if i == 0 {
			txt = "0.00ms"
		}
		out[i] = WaveAxisLabel{Frac: f, Text: txt}
	}
	return out
}

// waveWindowMs derives the wave tab's time window length from the
// number of samples and the active sample rate. Falls back to 44100
// Hz when the audio engine reports an unusable sample rate (test mode,
// pre-init). Returns 0 for empty buffers.
func waveWindowMs(numSamples int, sampleRate int) float64 {
	if numSamples <= 0 {
		return 0
	}
	sr := sampleRate
	if sr <= 0 {
		sr = 44100
	}
	return float64(numSamples) * 1000.0 / float64(sr)
}

// dbToFrac converts a dB value to a [0,1] fraction, clamped against the
// package-level meter floor/ceil constants.
func dbToFrac(db float64) float64 {
	if db <= meterDBFloor {
		return 0
	}
	if db >= meterDBCeil {
		return 1
	}
	return (db - meterDBFloor) / (meterDBCeil - meterDBFloor)
}

// truncateName shortens name so it fits within maxPx at the given scale.
// Uses TextWidth (Ebiten font metrics, with a stub fallback in tests).
func truncateName(name string, maxPx int, scale float64) string {
	if int(float64(TextWidth(name))*scale) <= maxPx {
		return name
	}
	for i := len(name) - 1; i > 0; i-- {
		candidate := name[:i]
		if int(float64(TextWidth(candidate))*scale) <= maxPx {
			return candidate
		}
	}
	return name[:1]
}

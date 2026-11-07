package ui

import (
	"fmt"
	"math"
)

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

func formatWindowMs(ms float64) string {
	if ms >= 10 {
		return fmt.Sprintf("%.0fms", ms)
	}
	if ms >= 1 {
		return fmt.Sprintf("%.1fms", ms)
	}
	return fmt.Sprintf("%.2fms", ms)
}

type WaveAxisLabel struct {
	Frac float64
	Text string
}

// The leftmost label always reads "0.00ms" to anchor the origin, as on Chain.
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

// Falls back to 44100 Hz when the engine reports an unusable rate (test mode, pre-init).
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

func dbToFrac(db float64) float64 {
	if db <= meterDBFloor {
		return 0
	}
	if db >= meterDBCeil {
		return 1
	}
	return (db - meterDBFloor) / (meterDBCeil - meterDBFloor)
}

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

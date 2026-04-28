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

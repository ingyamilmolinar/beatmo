package ui

import (
	"fmt"
	"math"
)

// timeToSteps returns the absolute subdivision index of the closest beat AT OR
// AFTER `seconds`, clamped to [0, maxSteps].
func timeToSteps(seconds float64, bpm, div, maxSteps int) int {
	if bpm <= 0 {
		bpm = 120
	}
	if div <= 0 {
		div = 1
	}
	beat := int(math.Ceil(seconds * float64(bpm) / 60.0))
	steps := beat * div
	if steps < 0 {
		steps = 0
	}
	// Clamp to [0, maxSteps]. maxSteps == 0 (an empty timeline) clamps to 0;
	// a negative maxSteps (never produced by timelineBeats*div) is treated as
	// "no cap".
	if maxSteps >= 0 && steps > maxSteps {
		steps = maxSteps
	}
	return steps
}

// formatClock renders whole seconds as "M:SS" (no clamp/negative handling —
// callers pass a non-negative playhead-derived second count).
func formatClock(sec int) string {
	if sec < 0 {
		sec = 0
	}
	return fmt.Sprintf("%d:%02d", sec/60, sec%60)
}

// formatBeatTime renders "Beat <b> · <m:ss>" for a position readout. It takes
// the resolved PlayheadPos rather than raw clock quantities so it cannot invent
// its own beat/second derivation — the beat counter renders the same two fields
// through formatBeatReadout (which drops the "Beat" label on mobile's narrow
// band), so the two readouts always agree on the numbers.
func formatBeatTime(p PlayheadPos) string {
	return fmt.Sprintf("Beat %d · %s", p.DisplayBeat, formatElapsedTime(p.Seconds))
}

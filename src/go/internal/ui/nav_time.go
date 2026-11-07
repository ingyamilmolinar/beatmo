package ui

import (
	"fmt"
	"math"
)

// AT OR AFTER seconds.
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
	// A negative maxSteps (never produced) means no cap.
	if maxSteps >= 0 && steps > maxSteps {
		steps = maxSteps
	}
	return steps
}

// Callers pass non-negative seconds.
func formatClock(sec int) string {
	if sec < 0 {
		sec = 0
	}
	return fmt.Sprintf("%d:%02d", sec/60, sec%60)
}

// Takes the resolved PlayheadPos so it cannot derive its own numbers: it always agrees with the beat counter.
func formatBeatTime(p PlayheadPos) string {
	return fmt.Sprintf("Beat %d · %s", p.DisplayBeat, formatElapsedTime(p.Seconds))
}

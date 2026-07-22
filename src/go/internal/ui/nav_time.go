package ui

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// parseClock parses "M:SS" or a bare seconds count into seconds. Seconds field
// must be 0..59.
func parseClock(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	if i := strings.IndexByte(s, ':'); i >= 0 {
		m, err1 := strconv.Atoi(strings.TrimSpace(s[:i]))
		sec, err2 := strconv.Atoi(strings.TrimSpace(s[i+1:]))
		if err1 != nil || err2 != nil || m < 0 || sec < 0 || sec > 59 {
			return 0, false
		}
		return float64(m*60 + sec), true
	}
	v, err := strconv.Atoi(s)
	if err != nil || v < 0 {
		return 0, false
	}
	return float64(v), true
}

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

// formatBeatTime renders "Beat <b> · <m:ss>" for a position readout.
func formatBeatTime(steps, div, bpm int) string {
	if div <= 0 {
		div = 1
	}
	if bpm <= 0 {
		bpm = 120
	}
	beat := steps / div
	sec := int(math.Round(float64(beat) * 60.0 / float64(bpm)))
	return fmt.Sprintf("Beat %d · %d:%02d", beat, sec/60, sec%60)
}

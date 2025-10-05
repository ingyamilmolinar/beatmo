//go:build js

package ui

import "math"

// On browsers, wheel deltas are often much larger (e.g., ~53). Normalize to
// approximate desktop feel by scaling down by ~50 and allowing fractional
// accumulation for smooth trackpads.
func wheelZoomDelta() float64 {
	_, wy := wheel()
	return wy * 0.02 // ~= 1/50 per notch
}

// Convert wheel delta to discrete row scroll steps. Ensure at least +/-1 per
// notch and cap extremes to avoid jumps from large deltas on some platforms.
func wheelScrollSteps() int {
	_, wy := wheel()
	s := int(math.Round(wy * 0.02))
	if s == 0 && wy != 0 { // ensure a notch registers
		if wy > 0 {
			s = 1
		} else {
			s = -1
		}
	}
	if s > 3 {
		s = 3
	}
	if s < -3 {
		s = -3
	}
	return s
}

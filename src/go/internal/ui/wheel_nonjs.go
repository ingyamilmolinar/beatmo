//go:build !js

package ui

// wheelZoomDelta normalizes the platform wheel delta for zoom operations to
// produce a similar feel across desktop environments.
func wheelZoomDelta() float64 {
	_, wy := wheel()
	return wy
}

// wheelStepsFromDelta converts a single wheel-axis delta into discrete scroll
// steps. Desktop wheels typically report +/-1 per notch; we clamp to small
// integers.
func wheelStepsFromDelta(d float64) int {
	if d > 0.5 {
		return 1
	}
	if d < -0.5 {
		return -1
	}
	return 0
}

// wheelScrollSteps converts the vertical wheel delta into discrete row scroll
// steps.
func wheelScrollSteps() int {
	_, wy := wheel()
	return wheelStepsFromDelta(wy)
}

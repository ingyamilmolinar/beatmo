//go:build !js

package ui

// wheelZoomDelta normalizes the platform wheel delta for zoom operations to
// produce a similar feel across desktop environments.
func wheelZoomDelta() float64 {
	_, wy := wheel()
	return wy
}

// wheelScrollSteps converts the wheel delta into discrete scroll steps (rows).
// Desktop wheels typically report +/-1 per notch; we clamp to small integers.
func wheelScrollSteps() int {
	_, wy := wheel()
	if wy > 0.5 {
		return 1
	}
	if wy < -0.5 {
		return -1
	}
	return 0
}

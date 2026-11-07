//go:build !js

package ui

// This file and wheel_js.go are the only readers of the raw wheel; everything above consumes detents.

// GLFW already reports ±1 per detent and accumulates the frame's events, so the conversion is the identity.
func wheelNotches() (x, y float64) {
	wx, wy := wheel()
	return wheelNotchesFromRaw(wx, WheelDeltaNotch), wheelNotchesFromRaw(wy, WheelDeltaNotch)
}

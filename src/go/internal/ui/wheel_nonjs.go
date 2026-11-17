//go:build !js

package ui

// wheel_nonjs.go — the NATIVE half of the platform wheel boundary.
// See input_dynamics.go for the contract. This file and wheel_js.go are the
// only places allowed to read the raw wheel; everything above consumes detents.

// wheelNotches returns this frame's wheel movement in DETENTS.
//
// GLFW already reports +/-1 per physical detent and ACCUMULATES every event in
// the frame (ebiten/internal/ui/input_glfw.go: `WheelY += yoff`), so the raw
// value is already the unit we want and the conversion is the identity.
func wheelNotches() (x, y float64) {
	wx, wy := wheel()
	return wheelNotchesFromRaw(wx, WheelDeltaNotch), wheelNotchesFromRaw(wy, WheelDeltaNotch)
}

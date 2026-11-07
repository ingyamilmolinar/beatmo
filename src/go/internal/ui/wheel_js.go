//go:build js

package ui

import "syscall/js"

// Not ebiten.Wheel(): its JS backend keeps only the last event of a frame and drops deltaMode (a Chrome notch vs a
// Firefox one differs ~33×). index.html accumulates raw deltas per frame; reading is PURE, since several sites read per frame.

// wheelNotches returns this frame's wheel movement in DETENTS.
func wheelNotches() (x, y float64) {
	g := js.Global().Get("__beatmoWheel")
	if !g.Truthy() {
		wx, wy := wheel() // fallback: Ebiten's raw last-event delta
		return wheelNotchesFromRaw(wx, WheelDeltaPixel), wheelNotchesFromRaw(wy, WheelDeltaPixel)
	}
	mode := WheelDeltaMode(g.Get("mode").Int())
	// DOM deltaY > 0 scrolls DOWN; negate so sign handling matches native.
	rx := -g.Get("x").Float()
	ry := -g.Get("y").Float()
	return wheelNotchesFromRaw(rx, mode), wheelNotchesFromRaw(ry, mode)
}

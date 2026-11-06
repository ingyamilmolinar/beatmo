//go:build js

package ui

import "syscall/js"

// wheel_js.go — the BROWSER half of the platform wheel boundary.
// See input_dynamics.go for the contract.
//
// This does NOT read ebiten.Wheel(). Ebiten's JS backend
// (internal/ui/input_js.go) does two things that break wheel handling:
//
//  1. it ASSIGNS `WheelY = -deltaY` per event, so when several wheel events
//     land in one frame only the last survives — fast scrolls lose motion;
//  2. it DISCARDS deltaMode, the only way to tell a Chrome notch (~100 px)
//     from a Firefox notch (~3 lines) — a ~33x difference. Ebiten even carries
//     an upstream "TODO: What if e.deltaMode is not DOM_DELTA_PIXEL?" there.
//
// Instead a capture-phase listener in index.html accumulates the raw deltas and
// records deltaMode, double-buffered on requestAnimationFrame so the published
// value is STABLE for the whole frame.
//
// Reading is therefore PURE — it must not consume. Several call sites read the
// wheel in one frame (grid dispatcher, drum row-scroll fallback, tree, camera);
// a draining read would starve whichever ran second. This mirrors the native
// contract, where ebiten.Wheel() can be read repeatedly within a frame.
//
// Per the JS Test Charter the listener is dumb transport; every conversion
// decision lives in Go (wheelNotchesFromRaw), where it is unit-tested.
//
// If the shim is missing (a stale cached index.html, or a harness that loads
// the wasm without the page), this falls back to ebiten.Wheel() read as pixels:
// the wheel keeps working with Chrome-ish calibration rather than dying.

// wheelNotches returns this frame's wheel movement in DETENTS.
func wheelNotches() (x, y float64) {
	g := js.Global().Get("__beatmoWheel")
	if !g.Truthy() {
		wx, wy := wheel() // fallback: Ebiten's raw last-event delta
		return wheelNotchesFromRaw(wx, WheelDeltaPixel), wheelNotchesFromRaw(wy, WheelDeltaPixel)
	}
	mode := WheelDeltaMode(g.Get("mode").Int())
	// The DOM's axes point the opposite way to Ebiten's (deltaY > 0 scrolls
	// DOWN); negate so downstream sign handling is identical on both platforms.
	rx := -g.Get("x").Float()
	ry := -g.Get("y").Float()
	return wheelNotchesFromRaw(rx, mode), wheelNotchesFromRaw(ry, mode)
}

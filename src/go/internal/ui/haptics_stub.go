//go:build !js || test

package ui

// platformVibrate is the build-tagged platform implementation of haptic
// feedback. The non-WASM (and WASM-test) implementation is a no-op so
// desktop builds don't pull in browser APIs and unit tests stay
// deterministic. Tests assert haptics by replacing `hapticEmit` (the
// upstream test seam) instead.
func platformVibrate(ms int) {
	_ = ms
}

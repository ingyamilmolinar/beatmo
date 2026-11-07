//go:build !js || test

package ui

// A no-op off WASM so desktop pulls in no browser API and tests stay deterministic.
func platformVibrate(ms int) {
	_ = ms
}

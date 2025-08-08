//go:build !js || !wasm

package audio

// Play is a stub for non-wasm builds.
func Play(id string, when float64) {}

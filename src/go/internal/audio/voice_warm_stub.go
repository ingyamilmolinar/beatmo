//go:build test && !js

package audio

// warmInstrumentPlatform is a no-op under the fast (-tags test) build: the C
// renderer is stubbed there and UI tests observe the WarmInstrument seam
// directly rather than the render side effect.
func warmInstrumentPlatform(id string, pitches []int) {}

//go:build test && !js

package audio

import "github.com/ingyamilmolinar/beatmo/internal/instrument"

// warmInstrumentPlatform is a no-op under the fast (-tags test) build: the C
// renderer is stubbed there and UI tests observe the WarmInstrument seam
// directly rather than the render side effect.
func warmInstrumentPlatform(id instrument.ID, pitches []int) {}

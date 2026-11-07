//go:build test && !js

package audio

import "github.com/ingyamilmolinar/beatmo/internal/instrument"

// UI tests observe the WarmInstrument seam directly.
func warmInstrumentPlatform(id instrument.ID, pitches []int) {}

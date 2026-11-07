//go:build test || js

package synthdiag

import (
	"errors"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// A matrix derived from the stand-in synth would be worse than no answer; this exists so cmd/refs still compiles.
func Ablate(instID instrument.ID, pitch float64, sr int, durSec float64, base map[string]float64) (Report, error) {
	return Report{}, errors.New("synthdiag: ablation needs the real synth build (not -tags test / js)")
}

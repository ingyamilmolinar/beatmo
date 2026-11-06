//go:build test || js

package synthdiag

import (
	"errors"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// ablate_stub.go — the fast-path / browser build of Ablate.
//
// Ablation is only meaningful against the SHIPPED C synth: under `-tags test`
// synthmatch renders a small pure-Go stand-in whose stage sensitivities are
// properties of the stub, not of the engine, and a plausible-looking matrix
// derived from it would be worse than no answer. So this build refuses rather
// than fabricating one -- it exists so cmd/refs's fast-path package still
// compiles with the synthdiag import.
func Ablate(instID instrument.ID, pitch float64, sr int, durSec float64, base map[string]float64) (Report, error) {
	return Report{}, errors.New("synthdiag: ablation needs the real synth build (not -tags test / js)")
}

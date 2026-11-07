package songrender

import "github.com/ingyamilmolinar/beatmo/internal/instrument"

// Mono float64 at Arrangement.SampleRate. The test build renders a deterministic pure-Go synth (master == Σ stems);
// the real build is the faithful engine mix.
type Rendered struct {
	Arrangement Arrangement
	Master      []float64
	Stems       map[instrument.ID][]float64
}

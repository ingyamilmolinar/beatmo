package songrender

// Rendered is the offline render of a template: the full arrangement, the master
// mix, and per-instrument stems (keyed by instrument id). Sample slices are mono
// float64 at Arrangement.SampleRate. Render is build-tag split:
//   - render_stub.go  (//go:build test):  deterministic pure-Go synth, master == Σ stems.
//   - render_prod.go  (//go:build !test): faithful audio-engine mix (vol/pan/sends/master).
type Rendered struct {
	Arrangement Arrangement
	Master      []float64
	Stems       map[string][]float64
}

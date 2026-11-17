//go:build test || js

package audio

import "github.com/ingyamilmolinar/beatmo/internal/instrument"

// instanceAlreadyRegistered reports whether id is in the playable instrument
// registry. Under the test/wasm builds the registry is a flat id list (Register
// ignores the Instrument value), so membership is a list scan via Instruments().
func instanceAlreadyRegistered(id instrument.ID) bool {
	for _, existing := range Instruments() {
		if existing == id {
			return true
		}
	}
	return false
}

// registerInstanceAlias adds id to the flat id registry. The test/wasm Register
// ignores its Instrument argument (no per-id Go renderer exists in these
// builds), so nil is correct; render equivalence in these builds is supplied
// entirely by the recipe binding + seeding the caller performs (wasm) or by the
// availability bookkeeping the test exercises.
func registerInstanceAlias(id, base instrument.ID) {
	Register(id, nil)
}

// registerDocInstrument adds a document instrument to the flat id registry
// (its block reaches audio.js through SetInstrumentDoc's platform push).
func registerDocInstrument(id instrument.ID) {
	Register(id, nil)
}

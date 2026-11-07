//go:build test || js

package audio

import "github.com/ingyamilmolinar/beatmo/internal/instrument"

// Under test/wasm the registry is a flat id list.
func instanceAlreadyRegistered(id instrument.ID) bool {
	for _, existing := range Instruments() {
		if existing == id {
			return true
		}
	}
	return false
}

// Register ignores the Instrument here (no Go renderer in these builds), so nil is correct.
func registerInstanceAlias(id, base instrument.ID) {
	Register(id, nil)
}

// Its block reaches audio.js through SetInstrumentDoc's platform push.
func registerDocInstrument(id instrument.ID) {
	Register(id, nil)
}

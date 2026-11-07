//go:build !test && !js

package audio

import "github.com/ingyamilmolinar/beatmo/internal/instrument"

func instanceAlreadyRegistered(id instrument.ID) bool {
	instMu.RLock()
	_, ok := instruments[id]
	instMu.RUnlock()
	return ok
}

// The shared Instrument is read-only at trigger time, so aliasing rather than cloning is safe and byte-identical.
func registerInstanceAlias(id, base instrument.ID) {
	instMu.RLock()
	inst, ok := instruments[base]
	instMu.RUnlock()
	if !ok {
		return
	}
	Register(id, inst)
}

// registerDocInstrument makes id play its own project document.
func registerDocInstrument(id instrument.ID) {
	Register(id, docInstrument{id: id})
}

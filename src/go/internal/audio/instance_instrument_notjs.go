//go:build !test && !js

package audio

import "github.com/ingyamilmolinar/beatmo/internal/instrument"

// instanceAlreadyRegistered reports whether id is in the playable instrument
// registry (the map keyed by id → Instrument).
func instanceAlreadyRegistered(id instrument.ID) bool {
	instMu.RLock()
	_, ok := instruments[id]
	instMu.RUnlock()
	return ok
}

// registerInstanceAlias makes id share the base instrument's playable Instrument
// so legacyNewVoice / instrumentDurationSamples resolve it identically. The
// shared Instrument value is read-only at trigger time, so aliasing (not
// cloning) is safe and guarantees byte-identical legacy renders; the recipe
// path (tryRecipeVoice) is driven by the binding + seeding done by the caller.
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

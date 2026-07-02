//go:build !test && !js

package audio

// instanceAlreadyRegistered reports whether id is in the playable instrument
// registry (the map keyed by id → Instrument).
func instanceAlreadyRegistered(id string) bool {
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
func registerInstanceAlias(id, base string) {
	instMu.RLock()
	inst, ok := instruments[base]
	instMu.RUnlock()
	if !ok {
		return
	}
	Register(id, inst)
}

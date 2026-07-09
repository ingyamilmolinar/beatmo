//go:build !test && !js

package audio

// registerClonedVoice makes newID a playable native voice that renders the
// cloned config by baking the effective seed (bakedModularRender), mirroring how
// the shipped seeded instruments are registered. The clone inherits the source
// voice's render window (Beats) so buffer-length-sensitive envelopes (e.g. the
// modal kick's buffer-normalized fade) survive the clone. Falls back to a
// source alias when the source isn't a CVariantInstrument (non-modular voices).
func registerClonedVoice(newID string, seed RecipeParams, srcID string) {
	beats := 0.5
	instMu.RLock()
	src, ok := instruments[srcID]
	instMu.RUnlock()
	if cv, isCV := src.(CVariantInstrument); isCV {
		beats = cv.Beats
	} else if ok {
		// Non-CVariant source: alias it (availability + fallback path).
		registerInstanceAlias(newID, srcID)
		return
	}
	Register(newID, CVariantInstrument{
		Name:   newID,
		Render: bakedModularRender(seed),
		Beats:  beats,
	})
}

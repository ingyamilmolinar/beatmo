package audio

import "math"

// InstrumentPitchInfo reports the effective pitch offset (in semitones from
// A3 = 220 Hz, the node-pitch-0 convention) contributed by an instrument's
// current synth + sampler parameters, and whether the instrument is pitched
// (musically noteworthy) rather than unpitched percussion.
//
// It is the note-display companion to the render path: a node's absolute note
// is noteName(A3 + nodePitch + offsetSemis). Because it reads the live param +
// sampler-edit state, the note derived from it tracks synth/sampler edits
// automatically — no cached value to keep in sync.
//
// Node pitch itself is NOT folded in here: at render time the node's semitone
// pitch overrides the recipe's generic "pitch" param (synth_recipe_dispatch.go
// merged["pitch"] = pitch), so the caller adds node pitch exactly once on top
// of this offset. The knobs folded here are the ones that STACK on top of the
// A3 base: octave, fine-detune (cents), and a base-frequency override.
//
// Performance: this is zero-allocation and holds each lock only briefly, so it
// is safe to call every frame from the UI draw loop. It reads the specific
// pitch keys directly from the params map without cloning it (unlike
// GetInstrumentParams, which returns a defensive copy).
func InstrumentPitchInfo(instrumentID string) (offsetSemis float64, pitched bool) {
	m := instrumentParamsMgr
	m.mu.RLock()
	p := m.params[instrumentID] // may be nil; reads on a nil map yield zero values
	recipeID := m.bindings[instrumentID]
	// Octave knob: whole octaves → ×12 semitones (default 0 = no shift).
	offsetSemis += p["osc_octave"] * 12
	// Fine detune in cents (osc_detune or the generic "tune"); default 0.
	offsetSemis += (p["osc_detune"] + p["tune"]) / 100.0
	// A base-frequency override replaces the 220 Hz (A3) base. The three keys
	// are mutually exclusive across recipe families; 0 means "use the built-in
	// A3 default", i.e. no offset.
	baseFreq := firstPositive(p["fm_base_freq"], p["fundamental"], p["base_freq"])
	m.mu.RUnlock()
	if baseFreq > 0 {
		offsetSemis += 12 * math.Log2(baseFreq/220.0)
	}

	// Sampler transpose/detune, if the instrument carries a sample-edit
	// descriptor (a non-destructive resample applied at trigger time).
	hasSampleEdit := false
	if e, ok := SampleEditFor(instrumentID); ok {
		offsetSemis += e.TransposeSemis + e.DetuneCents/100.0
		hasSampleEdit = true
	}

	// An instrument is "pitched" (worth a musical note name) exactly when the
	// engine treats it as melodic: pitchAwareRecipe is the same curated set the
	// render path uses to re-render at the node's semitone pitch (bowed/plucked
	// strings, keys, woodwinds, brass, bass, organ, sax). Drums, FM, the
	// configurable kicks (modular-category but percussive), congas, and pads are
	// deliberately excluded — a note name on them would mislead. A user sample
	// the player has transposed is also pitched. Keeping this in lockstep with
	// pitchAwareRecipe guarantees the displayed note matches what actually
	// sounds.
	pitched = pitchAwareRecipe(recipeID) || hasSampleEdit
	return offsetSemis, pitched
}

// firstPositive returns the first strictly-positive argument, or 0 if none is.
func firstPositive(vals ...float64) float64 {
	for _, v := range vals {
		if v > 0 {
			return v
		}
	}
	return 0
}

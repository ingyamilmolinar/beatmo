package audio

import "github.com/ingyamilmolinar/beatmo/internal/instrument"

// EnsureInstanceInstrument promotes an instance variant id (e.g. "organ--2") to
// a first-class, available, recipe-bound, voiced instrument that renders
// IDENTICALLY to its base ("organ"). The gen-showcase migration auto-suffixes
// repeated instrument ids in templates so each voice can carry its own display
// name; without this promotion the variant id is in none of the playable
// registry, the catalog, or roster.All, so it would be unavailable
// (import fails) and silent (no voice, no recipe).
//
// Behavior:
//   - idempotent: already-registered id → return.
//   - base == id (no instance suffix) → return.
//   - base not a registered instrument → return (nothing to alias).
//   - a synth base: the variant gets its OWN document copy of the base's
//     library entry (Origin = base) and plays it — same sound as the base.
//   - otherwise (a sample base): alias the variant onto the base's playable
//     Instrument with the base's amplitude.
//
// Mutates the PROCESS-GLOBAL instrument registry; tests must t.Cleanup with
// ResetInstruments.
func EnsureInstanceInstrument(id instrument.ID) {
	if instanceAlreadyRegistered(id) {
		return
	}
	base := id.Base()
	if base == id {
		return
	}
	if !instanceAlreadyRegistered(base) {
		return
	}
	// A synth instance gets its OWN document copy (spec §6) and plays it.
	if _, ok := EnsureInstrumentDoc(id); ok {
		registerDocInstrument(id)
		return
	}
	registerInstanceAlias(id, base)
	// Inherit the base's loudness-normalized amplitude (a non-document
	// instance has no render.amplitude of its own) — without this it would
	// fall back to DefaultAmplitude and render at the wrong level relative to
	// its base (same failure mode CloneInstrument guards against in
	// instrument_clone.go, which this mirrors).
	setRuntimeLoudnessAmp(id, ConfigForInstrument(base).Amplitude)
}

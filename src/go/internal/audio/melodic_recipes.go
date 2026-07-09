package audio

// melodicRecipeIDs is the set of synth-modular recipe IDs that represent
// melodic instruments (bowed/plucked strings, keys, woodwinds, brass, bass,
// organ, sax). These are pitch-aware: at trigger time the C renderer re-renders
// the voice at the node's semitone pitch so the filter formant stays at an
// absolute Hz (not resampled). Instruments NOT in this set (drums, FM, the
// configurable kicks, congas, synth-modular base, synth-modular-pad) keep the
// legacy resample path and are treated as unpitched percussion for note display.
//
// This lives in an untagged file (not synth_recipe_dispatch.go, which is
// //go:build !test && !js) so both the native render path and the UI
// note-display path (InstrumentPitchInfo, compiled under every build tag) read
// one source of truth — the displayed note is exactly what the engine sounds.
var melodicRecipeIDs = map[string]bool{
	// Bowed strings
	"synth-modular-violin":          true,
	"synth-modular-violin-ensemble": true,
	"synth-modular-cello":           true,
	"synth-modular-cello-warm":      true,
	"synth-modular-organ-church":    true,
	"synth-modular-scifi-lead":      true,
	// Plucked strings
	"synth-modular-guitar-nylon":         true,
	"synth-modular-guitar-nylon-bright":  true,
	"synth-modular-guitar-steel":         true,
	"synth-modular-guitar-steel-warm":    true,
	"synth-modular-guitar-electric":      true,
	"synth-modular-harp":                 true,
	"synth-modular-guitar-electric-neck": true,
	// Keys
	"synth-modular-piano-grand": true,
	"synth-modular-piano-felt":  true,
	// Woodwinds
	"synth-modular-flute":         true,
	"synth-modular-flute-breathy": true,
	"synth-modular-oboe":          true,
	"synth-modular-oboe-full":     true,
	// Brass
	"synth-modular-trumpet":          true,
	"synth-modular-trumpet-mellow":   true,
	"synth-modular-french-horn":      true,
	"synth-modular-french-horn-loud": true,
	// Bass guitar (renamed from synth-bass) + synth bass family — pitched melodic.
	"synth-modular-bass-guitar": true,
	"synth-modular-bass-acid":   true,
	"synth-modular-bass-reese":  true,
	"synth-modular-bass-fm":     true,
	"synth-modular-bass-808":    true,
	// Masterpiece template set — pitched melodic instruments.
	"synth-modular-organ": true,
	"synth-modular-sax":   true,
	// Voice/choir family — pitched vocal instruments (whisper is unpitched).
	// ensemble-lead(-dark)/ghost-bass/viola-pad were built as voice/choir
	// attempts and re-categorized as synths per ear review 2026-07-08 (same
	// recipe ids, still pitch-aware — only the id/category moved).
	"synth-modular-ensemble-lead":      true,
	"synth-modular-ensemble-lead-dark": true,
	"synth-modular-voice-soprano":      true,
	"synth-modular-ghost-bass":         true,
	"synth-modular-viola-pad":          true,
	"synth-modular-voice-ahh":          true,
	"synth-modular-voice-opera":        true,
}

// pitchAwareRecipe returns true when recipeID is a melodic synth-modular recipe
// that benefits from per-pitch re-rendering instead of resampling. Returns
// false for drums, FM, the configurable kicks, synth-modular (base), and
// synth-modular-pad.
func pitchAwareRecipe(recipeID string) bool {
	return melodicRecipeIDs[recipeID]
}

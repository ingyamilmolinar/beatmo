//go:build !test && !js

package audio

// kick_modular_native.go holds the native (CGo) no-edit fast-path renderers for
// the migrated kick family. After the Phase-3 cutover, the instruments-table
// Render closures for the five kick instruments no longer call the deleted
// bespoke render_kick / render_kick_deep / render_kick_punchy / render_kick_lofi
// / render_kick_tight C functions; instead they bake the recipe-default
// ModularParams ONCE (via the same kickRecipeToModular binding the recipe/edit
// path uses) and render them through render_modular_p. This keeps the no-edit
// dispatch path byte-identical to the recipe path for an unedited instrument —
// the exact renderModularPad pattern (modular_c.go), mirroring
// bass_modular_native.go.
//
// The baked params are built from the recipe DEFAULTS merged over an empty
// overlay, so they reproduce the exact param block the recipe path renders for
// an instrument the user has never touched. TestKickNoEditFastPathMatchesRecipe
// (kick_native_fastpath_test.go) locks the byte-identity; the existing
// nativeGoldenCases kick entries (re-pointed at these renderers via
// kickGoldenIdentity) prove the hash is unchanged from the deleted legacy path.

// kickVoiceParamsFor bakes the recipe-default ModularParams for one migrated
// kick recipe through the production binding (kickRecipeToModular), reading the
// per-variant spec (variant code, fundamental default, wired curated knobs) from
// kickVariantSpecs — the single source of truth shared with the push.
func kickVoiceParamsFor(recipeID string) ModularParams {
	spec := kickVariantSpecs[recipeID]
	return kickRecipeToModular(
		recipeID,
		MergeRecipeDefaults(recipeID, RecipeParams{}),
		spec.variant,
		spec.def,
		kickWiredFields(recipeID),
	)
}

// The five baked no-edit kick voices, built once at init from the recipe
// defaults through the migration binding (mirroring subBassVoiceParams /
// modularPadVoiceParams).
var (
	kickVoiceParams       = kickVoiceParamsFor("drum-kick")
	kickDeepVoiceParams   = kickVoiceParamsFor("drum-kick-deep")
	kickPunchyVoiceParams = kickVoiceParamsFor("drum-kick-punchy")
	kickLofiVoiceParams   = kickVoiceParamsFor("drum-kick-lofi")
	kickTightVoiceParams  = kickVoiceParamsFor("drum-kick-tight")
)

// renderKickVoice renders the base kick no-edit voice through the modular engine
// with its baked recipe-default params (replaces the deleted render_kick legacy
// fast path).
func renderKickVoice(buf []float32, sampleRate, samples int) {
	if len(buf) == 0 || samples == 0 || samples > len(buf) {
		return
	}
	renderModularP(buf, sampleRate, samples, kickVoiceParams)
}

// renderKickDeepVoice renders the deep kick no-edit voice (replaces
// render_kick_deep).
func renderKickDeepVoice(buf []float32, sampleRate, samples int) {
	if len(buf) == 0 || samples == 0 || samples > len(buf) {
		return
	}
	renderModularP(buf, sampleRate, samples, kickDeepVoiceParams)
}

// renderKickPunchyVoice renders the punchy kick no-edit voice (replaces
// render_kick_punchy).
func renderKickPunchyVoice(buf []float32, sampleRate, samples int) {
	if len(buf) == 0 || samples == 0 || samples > len(buf) {
		return
	}
	renderModularP(buf, sampleRate, samples, kickPunchyVoiceParams)
}

// renderKickLofiVoice renders the lo-fi kick no-edit voice (replaces
// render_kick_lofi).
func renderKickLofiVoice(buf []float32, sampleRate, samples int) {
	if len(buf) == 0 || samples == 0 || samples > len(buf) {
		return
	}
	renderModularP(buf, sampleRate, samples, kickLofiVoiceParams)
}

// renderKickTightVoice renders the tight kick no-edit voice (replaces
// render_kick_tight).
func renderKickTightVoice(buf []float32, sampleRate, samples int) {
	if len(buf) == 0 || samples == 0 || samples > len(buf) {
		return
	}
	renderModularP(buf, sampleRate, samples, kickTightVoiceParams)
}

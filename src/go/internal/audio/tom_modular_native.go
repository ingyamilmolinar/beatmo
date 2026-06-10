//go:build !test && !js

package audio

// tom_modular_native.go holds the native (CGo) no-edit fast-path renderers for
// the migrated tom family. After the Phase-4 cutover, the instruments-table
// Render closures for the three tom instruments no longer call the deleted
// bespoke render_tom / render_tom_high / render_tom_low C functions; instead they
// bake the recipe-default ModularParams ONCE (via the same tomRecipeToModular
// binding the recipe/edit path uses) and render them through render_modular_p.
// This keeps the no-edit dispatch path byte-identical to the recipe path for an
// unedited instrument — the exact renderKickVoice / renderModularPad pattern,
// mirroring kick_modular_native.go.
//
// The baked params are built from the recipe DEFAULTS merged over an empty
// overlay, so they reproduce the exact param block the recipe path renders for an
// instrument the user has never touched. TestTomNoEditFastPathMatchesRecipe
// (tom_native_fastpath_test.go) locks the byte-identity; the existing
// nativeGoldenCases tom entries (re-pointed at these renderers via
// tomGoldenIdentity) prove the hash is unchanged from the deleted legacy path.

// tomVoiceParamsFor bakes the recipe-default ModularParams for one migrated tom
// recipe through the production binding (tomRecipeToModular), reading the
// per-variant spec (variant code, fundamental default) from tomVariantSpecs — the
// single source of truth shared with the push.
func tomVoiceParamsFor(recipeID string) ModularParams {
	spec := tomVariantSpecs[recipeID]
	return tomRecipeToModular(
		recipeID,
		MergeRecipeDefaults(recipeID, RecipeParams{}),
		spec.variant,
		spec.def,
	)
}

// The three baked no-edit tom voices, built once at init from the recipe defaults
// through the migration binding (mirroring kickVoiceParams / subBassVoiceParams).
var (
	tomVoiceParams     = tomVoiceParamsFor("drum-tom")
	tomHighVoiceParams = tomVoiceParamsFor("drum-tom-high")
	tomLowVoiceParams  = tomVoiceParamsFor("drum-tom-low")
)

// renderTomVoice renders the base tom no-edit voice through the modular engine
// with its baked recipe-default params (replaces the deleted render_tom legacy
// fast path).
func renderTomVoice(buf []float32, sampleRate, samples int) {
	if len(buf) == 0 || samples == 0 || samples > len(buf) {
		return
	}
	renderModularP(buf, sampleRate, samples, tomVoiceParams)
}

// renderTomHighVoice renders the high tom no-edit voice (replaces render_tom_high).
func renderTomHighVoice(buf []float32, sampleRate, samples int) {
	if len(buf) == 0 || samples == 0 || samples > len(buf) {
		return
	}
	renderModularP(buf, sampleRate, samples, tomHighVoiceParams)
}

// renderTomLowVoice renders the low tom no-edit voice (replaces render_tom_low).
func renderTomLowVoice(buf []float32, sampleRate, samples int) {
	if len(buf) == 0 || samples == 0 || samples > len(buf) {
		return
	}
	renderModularP(buf, sampleRate, samples, tomLowVoiceParams)
}

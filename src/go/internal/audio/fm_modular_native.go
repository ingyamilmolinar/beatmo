//go:build !test && !js

package audio

// fm_modular_native.go holds the native (CGo) no-edit fast-path renderers for the
// migrated FM family — the LAST legacy family. After the Phase-7 cutover, the
// instruments-table Render closures for the fm-bass/bell/lead/epiano/pluck
// instruments no longer call the deleted bespoke render_fm_bass / render_fm_bell
// / render_fm_lead / render_fm_epiano / render_fm_pluck C functions; instead they
// bake the recipe-default ModularParams ONCE (via the same fmRecipeToModular
// binding the recipe/edit path uses) and render them through render_modular_p.
// This keeps the no-edit dispatch path byte-identical to the recipe path for an
// unedited instrument — the exact cymbalVoiceParams / snareVoiceParams pattern.
//
// The baked params are built from the recipe DEFAULTS merged over an empty
// overlay, so they reproduce the exact param block the recipe path renders for an
// instrument the user has never touched. TestFMNoEditFastPathMatchesRecipe
// (fm_native_fastpath_test.go) locks the byte-identity; the FM preset golden
// (fm_golden_test.go, re-pointed at these baked voices) proves the hash is
// unchanged from the deleted legacy path.

// fmVoiceParamsFor bakes the recipe-default ModularParams for one migrated FM
// recipe through the production binding (fmRecipeToModular), reading the
// per-variant spec (variant code, wired curated knobs) from fmVariantSpecs — the
// single source of truth shared with the push.
func fmVoiceParamsFor(recipeID string) ModularParams {
	spec := fmVariantSpecs[recipeID]
	return fmRecipeToModular(
		recipeID,
		MergeRecipeDefaults(recipeID, RecipeParams{}),
		spec.variant,
		fmWiredFields(recipeID),
	)
}

// The five baked no-edit FM-family voices, built once at init from the recipe
// defaults through the migration binding (mirroring cymbalVoiceParams /
// snareVoiceParams).
var (
	fmBassVoiceParams   = fmVoiceParamsFor("fm-bass")
	fmBellVoiceParams   = fmVoiceParamsFor("fm-bell")
	fmLeadVoiceParams   = fmVoiceParamsFor("fm-lead")
	fmEPianoVoiceParams = fmVoiceParamsFor("fm-epiano")
	fmPluckVoiceParams  = fmVoiceParamsFor("fm-pluck")
)

// renderFMBassVoice renders the fm-bass no-edit voice through the modular engine
// with its baked recipe-default params (replaces the deleted render_fm_bass
// legacy fast path).
func renderFMBassVoice(buf []float32, sampleRate, samples int) {
	if len(buf) == 0 || samples == 0 || samples > len(buf) {
		return
	}
	renderModularP(buf, sampleRate, samples, fmBassVoiceParams)
}

// renderFMBellVoice renders the fm-bell no-edit voice (replaces render_fm_bell).
func renderFMBellVoice(buf []float32, sampleRate, samples int) {
	if len(buf) == 0 || samples == 0 || samples > len(buf) {
		return
	}
	renderModularP(buf, sampleRate, samples, fmBellVoiceParams)
}

// renderFMLeadVoice renders the fm-lead no-edit voice (replaces render_fm_lead).
func renderFMLeadVoice(buf []float32, sampleRate, samples int) {
	if len(buf) == 0 || samples == 0 || samples > len(buf) {
		return
	}
	renderModularP(buf, sampleRate, samples, fmLeadVoiceParams)
}

// renderFMEPianoVoice renders the fm-epiano no-edit voice (replaces
// render_fm_epiano).
func renderFMEPianoVoice(buf []float32, sampleRate, samples int) {
	if len(buf) == 0 || samples == 0 || samples > len(buf) {
		return
	}
	renderModularP(buf, sampleRate, samples, fmEPianoVoiceParams)
}

// renderFMPluckVoice renders the fm-pluck no-edit voice (replaces
// render_fm_pluck).
func renderFMPluckVoice(buf []float32, sampleRate, samples int) {
	if len(buf) == 0 || samples == 0 || samples > len(buf) {
		return
	}
	renderModularP(buf, sampleRate, samples, fmPluckVoiceParams)
}

//go:build !test && !js

package audio

// cymbal_modular_native.go holds the native (CGo) no-edit fast-path renderers for
// the migrated cymbal family. After the Phase-6 cutover, the instruments-table
// Render closures for the hihat/open-hihat/cowbell/shaker/ride/crash instruments
// no longer call the deleted bespoke render_hihat / render_open_hihat /
// render_cowbell / render_shaker / render_ride / render_crash C functions;
// instead they bake the recipe-default ModularParams ONCE (via the same
// cymbalRecipeToModular binding the recipe/edit path uses) and render them through
// render_modular_p. This keeps the no-edit dispatch path byte-identical to the
// recipe path for an unedited instrument — the exact snareVoiceParams /
// kickVoiceParams pattern.
//
// The baked params are built from the recipe DEFAULTS merged over an empty
// overlay, so they reproduce the exact param block the recipe path renders for an
// instrument the user has never touched. TestCymbalNoEditFastPathMatchesRecipe
// (cymbal_native_fastpath_test.go) locks the byte-identity; the existing
// nativeGoldenCases cymbal entries (re-pointed at these renderers via
// cymbalGoldenIdentity) prove the hash is unchanged from the deleted legacy path.

// cymbalVoiceParamsFor bakes the recipe-default ModularParams for one migrated
// cymbal recipe through the production binding (cymbalRecipeToModular), reading
// the per-variant spec (variant code, wired curated knobs) from
// cymbalVariantSpecs — the single source of truth shared with the push.
func cymbalVoiceParamsFor(recipeID string) ModularParams {
	spec := cymbalVariantSpecs[recipeID]
	return cymbalRecipeToModular(
		recipeID,
		MergeRecipeDefaults(recipeID, RecipeParams{}),
		spec.variant,
		cymbalWiredFields(recipeID),
	)
}

// The six baked no-edit cymbal-family voices, built once at init from the recipe
// defaults through the migration binding (mirroring snareVoiceParams /
// tomVoiceParams).
var (
	hihatVoiceParams     = cymbalVoiceParamsFor("drum-hihat")
	openHihatVoiceParams = cymbalVoiceParamsFor("drum-open-hihat")
	cowbellVoiceParams   = cymbalVoiceParamsFor("drum-cowbell")
	shakerVoiceParams    = cymbalVoiceParamsFor("drum-shaker")
	rideVoiceParams      = cymbalVoiceParamsFor("drum-ride")
	crashVoiceParams     = cymbalVoiceParamsFor("drum-crash")
)

// renderHiHatVoice renders the hihat no-edit voice through the modular engine
// with its baked recipe-default params (replaces the deleted render_hihat legacy
// fast path).
func renderHiHatVoice(buf []float32, sampleRate, samples int) {
	if len(buf) == 0 || samples == 0 || samples > len(buf) {
		return
	}
	renderModularP(buf, sampleRate, samples, hihatVoiceParams)
}

// renderOpenHiHatVoice renders the open-hihat no-edit voice (replaces
// render_open_hihat).
func renderOpenHiHatVoice(buf []float32, sampleRate, samples int) {
	if len(buf) == 0 || samples == 0 || samples > len(buf) {
		return
	}
	renderModularP(buf, sampleRate, samples, openHihatVoiceParams)
}

// renderCowbellVoice renders the cowbell no-edit voice (replaces render_cowbell).
func renderCowbellVoice(buf []float32, sampleRate, samples int) {
	if len(buf) == 0 || samples == 0 || samples > len(buf) {
		return
	}
	renderModularP(buf, sampleRate, samples, cowbellVoiceParams)
}

// renderShakerVoice renders the shaker no-edit voice (replaces render_shaker).
func renderShakerVoice(buf []float32, sampleRate, samples int) {
	if len(buf) == 0 || samples == 0 || samples > len(buf) {
		return
	}
	renderModularP(buf, sampleRate, samples, shakerVoiceParams)
}

// renderRideVoice renders the ride no-edit voice (replaces render_ride).
func renderRideVoice(buf []float32, sampleRate, samples int) {
	if len(buf) == 0 || samples == 0 || samples > len(buf) {
		return
	}
	renderModularP(buf, sampleRate, samples, rideVoiceParams)
}

// renderCrashVoice renders the crash no-edit voice (replaces render_crash).
func renderCrashVoice(buf []float32, sampleRate, samples int) {
	if len(buf) == 0 || samples == 0 || samples > len(buf) {
		return
	}
	renderModularP(buf, sampleRate, samples, crashVoiceParams)
}

//go:build !test && !js

package audio

// snare_modular_native.go holds the native (CGo) no-edit fast-path renderers for
// the migrated snare family. After the Phase-5 cutover, the instruments-table
// Render closures for the snare/rimshot/sidestick/clap instruments no longer call
// the deleted bespoke render_snare / render_snare_rimshot / render_snare_sidestick
// / render_clap C functions; instead they bake the recipe-default ModularParams
// ONCE (via the same snareRecipeToModular binding the recipe/edit path uses) and
// render them through render_modular_p. This keeps the no-edit dispatch path
// byte-identical to the recipe path for an unedited instrument — the exact
// renderKickVoice / renderTomVoice pattern.
//
// The baked params are built from the recipe DEFAULTS merged over an empty
// overlay, so they reproduce the exact param block the recipe path renders for an
// instrument the user has never touched. TestSnareNoEditFastPathMatchesRecipe
// (snare_native_fastpath_test.go) locks the byte-identity; the existing
// nativeGoldenCases snare entries (re-pointed at these renderers via
// snareGoldenIdentity) prove the hash is unchanged from the deleted legacy path.

// snareVoiceParamsFor bakes the recipe-default ModularParams for one migrated
// snare recipe through the production binding (snareRecipeToModular), reading the
// per-variant spec (variant code, fundamental default, wired curated knobs) from
// snareVariantSpecs — the single source of truth shared with the push.
func snareVoiceParamsFor(recipeID string) ModularParams {
	spec := snareVariantSpecs[recipeID]
	return snareRecipeToModular(
		recipeID,
		MergeRecipeDefaults(recipeID, RecipeParams{}),
		spec.variant,
		spec.def,
		snareWiredFields(recipeID),
	)
}

// The four baked no-edit snare-family voices, built once at init from the recipe
// defaults through the migration binding (mirroring kickVoiceParams /
// tomVoiceParams).
var (
	snareVoiceParams          = snareVoiceParamsFor("drum-snare")
	snareRimshotVoiceParams   = snareVoiceParamsFor("drum-snare-rimshot")
	snareSidestickVoiceParams = snareVoiceParamsFor("drum-snare-sidestick")
	clapVoiceParams           = snareVoiceParamsFor("drum-clap")
)

// renderSnareVoice renders the snare no-edit voice through the modular engine
// with its baked recipe-default params (replaces the deleted render_snare legacy
// fast path).
func renderSnareVoice(buf []float32, sampleRate, samples int) {
	if len(buf) == 0 || samples == 0 || samples > len(buf) {
		return
	}
	renderModularP(buf, sampleRate, samples, snareVoiceParams)
}

// renderSnareRimshotVoice renders the rimshot no-edit voice (replaces
// render_snare_rimshot).
func renderSnareRimshotVoice(buf []float32, sampleRate, samples int) {
	if len(buf) == 0 || samples == 0 || samples > len(buf) {
		return
	}
	renderModularP(buf, sampleRate, samples, snareRimshotVoiceParams)
}

// renderSnareSidestickVoice renders the sidestick no-edit voice (replaces
// render_snare_sidestick).
func renderSnareSidestickVoice(buf []float32, sampleRate, samples int) {
	if len(buf) == 0 || samples == 0 || samples > len(buf) {
		return
	}
	renderModularP(buf, sampleRate, samples, snareSidestickVoiceParams)
}

// renderClapVoice renders the clap no-edit voice (replaces render_clap).
func renderClapVoice(buf []float32, sampleRate, samples int) {
	if len(buf) == 0 || samples == 0 || samples > len(buf) {
		return
	}
	renderModularP(buf, sampleRate, samples, clapVoiceParams)
}

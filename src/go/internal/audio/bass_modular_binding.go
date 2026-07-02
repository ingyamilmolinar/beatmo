//go:build !test && !js

package audio

import "math"

// The migration marker is the tag-neutral modularMigrations registry (see
// family_modular_push.go): IsModularMigratedRecipe(id) is the in-package marker
// the migration tests assert against. The native rebind itself happens in init()
// below and additionally proves byte-routing via builtinFamilyRenderers[id].
//
// subBassFundamental / bassGuitarFundamental moved to bass_modular_push.go
// (build-tag-neutral) so the WASM push seam can resolve the fundamental too.

// bassRecipeToModular maps the legacy-named drum-sub-bass RecipeParams onto the
// modular engine's wide ModularParams block. It is the native render binding and
// (Phase 8) the future rename table.
//
// Byte-identity strategy: the analytic source==4 voice reads each family field
// via kp_get(field, legacy_literal) in C, so passing NaN reproduces the exact
// legacy double literal at default, while a non-default value arrives
// float32-quantized — exactly as the legacy NaN-elision path behaves. We
// therefore drive the family fields from the ELIDED params (default → absent →
// NaN), matching bassFamilyRenderer's elideRecipeDefaults(...) call. The generic
// wired knobs (pitch/decay/drive/body) feed the shared POST stage the same way
// the legacy render_sub_bass_p does (apply_post_params over the elided base).
func bassRecipeToModular(recipeID string, merged RecipeParams) ModularParams {
	elided := elideRecipeDefaults(recipeID, merged)
	nan := math.NaN()

	// Family-generic seed: legacy osc/amp/filter/drive stages OFF, gen slots off,
	// FMEnabled = ModularParamSchemaIdentity()["fm_enabled"] (unread; engine
	// default). The byte-identity rationale lives on modularIdentityBase.
	mp := modularStageBase(recipeID, merged)

	// Voice frequency = the resolved sub-bass fundamental (exact Hz override).
	mp.VoiceFreqHz = subBassFundamental(merged)

	// Slot 1 = analytic dual-osc voice (source==4), ratio mode ×1 → voice freq.
	// Family fields are driven from the ELIDED params via elidedValOrNaN: default
	// → absent → NaN, so the C analytic voice's kp_get(field, legacy_literal)
	// reproduces the exact legacy double literal at default while a non-default
	// value arrives float32-quantized (matches the legacy NaN-elision path).
	mp.GenSource[0] = 4
	mp.GenFreqMode[0] = 0 // ratio
	mp.GenFreq[0] = 1     // ×voice_freq
	mp.GenPhaseMode[0] = 1
	mp.GenPhase[0] = nan // → kp_get fallback M_PI*0.5 (exact double π/2)
	mp.GenWave[0] = elidedValOrNaN(elided, "bass_wave")
	mp.GenHarmMix[0] = elidedValOrNaN(elided, "bass_harmonic")
	mp.GenPitchEnvAmt[0] = elidedValOrNaN(elided, "bass_pitch_env")
	mp.GenPitchEnvRate[0] = nan // → 40.0
	mp.GenAtkAmt[0] = elidedValOrNaN(elided, "bass_attack")
	mp.GenAtkRate[0] = nan // → 60.0
	mp.GenEnvFastRate[0] = elidedValOrNaN(elided, "bass_env_rate")
	mp.GenSatK[0] = nan     // → 1.2 (double literal)
	mp.GenOutScale[0] = nan // → 0.95f (float literal)
	// Slots 2..12 stay at source 0 (off).
	for i := 1; i < modularGenSlots; i++ {
		mp.GenSource[i] = 0
	}

	// Shared POST stage: drum-sub-bass cfg {decay_rate=4, pitch,decay,drive,body
	// gates on; brightness,tone off} over the elided generic base. postSkipped is
	// the bass-family-specific NULL-base condition (the deleted BassParams.toC()
	// ==nil rule), now inlined in bassLegacyNilElision — see applyFamilyPostStage
	// for the byte-identity rationale.
	postSkipped := bassLegacyNilElision(elided)
	applyFamilyPostStage(&mp, elided, postSkipped, subBassPostConfig)

	return mp
}

func init() {
	// Rebind drum-sub-bass onto the modular engine. The renderer maps merged
	// RecipeParams → ModularParams and renders via render_modular_p, replacing
	// the legacy render_sub_bass_p path. The oracle fixtures prove byte-identity.
	// Guarded by registry membership (no third marker map): the binding only
	// rebinds recipes the tag-neutral modularMigrations registry owns.
	if IsModularMigratedRecipe("drum-sub-bass") {
		builtinFamilyRenderers["drum-sub-bass"] = func(buf []float32, sampleRate, samples int, p RecipeParams) {
			renderModularP(buf, sampleRate, samples, bassRecipeToModular("drum-sub-bass", p))
		}
	}
}

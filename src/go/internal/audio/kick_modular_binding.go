//go:build !test && !js

package audio

// kick_modular_binding.go is the native (CGo) render binding for the migrated
// kick family. Each kick recipe maps its legacy-named RecipeParams onto the
// modular engine's wide ModularParams block via the source==5 harmonic-bank
// kick voice; init() rebinds builtinFamilyRenderers[id] so NewRecipe(id).Render
// flows through render_modular_p. The oracle fixtures prove byte-identity.
//
// Byte-identity strategy mirrors bassRecipeToModular: the source==5 voice reads
// each curated kick knob via kp_get(field, legacy_literal) in C, so passing NaN
// reproduces the exact legacy double literal at default while a non-default
// value arrives float32-quantized — matching the legacy NaN-elision path. The
// per-variant kp_get literals live INSIDE the C voice (one per variant branch),
// so the binding passes NaN for every curated knob the variant wires and the C
// branch's literal supplies the value. The generic wired knobs feed the shared
// POST stage exactly like the legacy render_kick_p / render_kick_*_p.

// kickRecipeToModular maps a kick recipe's merged RecipeParams onto ModularParams
// via the source==5 voice. variant is the C discriminator (0=base 1=deep
// 2=punchy 3=lofi 4=tight); def is the per-variant fundamental default. The
// wired set tells which curated knobs the variant synthesizes — the binding
// passes elidedValOrNaN for those (so a default knob is NaN → the C kp_get
// literal) and leaves the rest at the schema identity (unread for that variant).
func kickRecipeToModular(recipeID string, merged RecipeParams, variant float64, def float64, wired map[string]bool) ModularParams {
	elided := elideRecipeDefaults(recipeID, merged)

	mp := modularStageBase(recipeID, merged)

	// Voice frequency = the resolved kick fundamental (exact Hz). The source==5
	// voice reads it through gen_freq×voice_freq (ratio mode).
	mp.VoiceFreqHz = kickFundamental(merged, def)

	mp.GenSource[0] = 5
	mp.GenFreqMode[0] = 0 // ratio
	mp.GenFreq[0] = 1     // ×voice_freq
	mp.GenKickVariant[0] = variant
	// kick_wave reuses gen_wave (elidedValOrNaN → C kp_get fallback 0.0).
	mp.GenWave[0] = elidedValOrNaN(elided, "kick_wave")

	// Curated knobs: drive each wired field from the elided params via
	// elidedValOrNaN (default → absent → NaN → the C kp_get per-variant literal).
	set := func(dst *float64, legacyName string) {
		if wired[legacyName] {
			*dst = elidedValOrNaN(elided, legacyName)
		}
	}
	set(&mp.GenKickH2[0], "kick_h2_gain")
	set(&mp.GenKickH3[0], "kick_h3_gain")
	set(&mp.GenKickH4[0], "kick_h4_gain")
	set(&mp.GenKickEnv0[0], "kick_env0_rate")
	set(&mp.GenKickEnv1[0], "kick_env1_rate")
	set(&mp.GenKickPeAmt[0], "kick_pitch_env_amount")
	set(&mp.GenKickPeRate[0], "kick_pitch_env_rate")
	set(&mp.GenKickClick[0], "kick_click")
	set(&mp.GenKickNoise[0], "kick_noise")

	// Slots 2..12 stay at source 0 (off).
	for i := 1; i < 12; i++ {
		mp.GenSource[i] = 0
	}

	// Shared POST stage: all five kick recipes use {decay_rate=6, pitch, decay,
	// body, drive on; tone, brightness off}. postSkipped is the kick-family
	// NULL-base condition (the deleted KickParams.toC()==nil rule), inlined in
	// kickLegacyNilElision.
	postSkipped := kickLegacyNilElision(elided)
	applyFamilyPostStage(&mp, elided, postSkipped, kickPostConfigFor(variant))

	return mp
}

// kickWiredFields returns the set of legacy curated-knob names a variant
// synthesizes (the keys of its kp_get-literal map). Built from kickVariantSpecs
// so the binding and the push share one source of truth.
func kickWiredFields(recipeID string) map[string]bool {
	spec := kickVariantSpecs[recipeID]
	out := make(map[string]bool, len(spec.lit))
	for name := range spec.lit {
		out[name] = true
	}
	return out
}

func init() {
	// Rebind every migrated kick recipe onto the modular engine. Guarded by
	// registry membership (the tag-neutral modularMigrations registry, populated
	// by registerKickMigrations's package-var init before this init runs).
	for recipeID, spec := range kickVariantSpecs {
		if !IsModularMigratedRecipe(recipeID) {
			continue
		}
		recipeID := recipeID
		variant := spec.variant
		def := spec.def
		wired := kickWiredFields(recipeID)
		builtinFamilyRenderers[recipeID] = func(buf []float32, sampleRate, samples int, p RecipeParams) {
			renderModularP(buf, sampleRate, samples, kickRecipeToModular(recipeID, p, variant, def, wired))
		}
	}
}

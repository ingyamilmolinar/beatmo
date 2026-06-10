//go:build !test && !js

package audio

// tom_modular_binding.go is the native (CGo) render binding for the migrated tom
// family. Each tom recipe maps its legacy-named RecipeParams onto the modular
// engine's wide ModularParams block via the source==6 808-style tom voice;
// init() rebinds builtinFamilyRenderers[id] so NewRecipe(id).Render flows through
// render_modular_p. The oracle fixtures prove byte-identity.
//
// Byte-identity strategy mirrors kickRecipeToModular: the source==6 voice reads
// each curated tom knob via kp_get(field, legacy_literal) in C, so passing NaN
// reproduces the exact legacy double literal at default while a non-default value
// arrives float32-quantized — matching the legacy NaN-elision path. The
// per-variant kp_get literals live INSIDE the C voice (selected by the variant
// discriminator), so the binding passes NaN for every curated knob and the C
// voice's variant-indexed literal table supplies the value. The generic wired
// knobs feed the shared POST stage exactly like the legacy render_tom_p /
// render_tom_high_p / render_tom_low_p.

// tomRecipeToModular maps a tom recipe's merged RecipeParams onto ModularParams
// via the source==6 voice. variant is the C discriminator (0=tom 1=high 2=low);
// def is the per-variant fundamental default.
func tomRecipeToModular(recipeID string, merged RecipeParams, variant float64, def float64) ModularParams {
	elided := elideRecipeDefaults(recipeID, merged)

	mp := modularStageBase(recipeID, merged)

	// Voice frequency = the resolved tom fundamental (exact Hz). The source==6
	// voice reads it through gen_freq×voice_freq (ratio mode).
	mp.VoiceFreqHz = tomFundamental(merged, def)

	mp.GenSource[0] = 6
	mp.GenFreqMode[0] = 0 // ratio
	mp.GenFreq[0] = 1     // ×voice_freq
	mp.GenTomVariant[0] = variant
	// tom_wave reuses gen_wave (elidedValOrNaN → C kp_get fallback 0.0).
	mp.GenWave[0] = elidedValOrNaN(elided, "tom_wave")

	// Curated knobs: drive each field from the elided params via elidedValOrNaN
	// (default → absent → NaN → the C kp_get per-variant literal).
	mp.GenTomSweep[0] = elidedValOrNaN(elided, "tom_sweep_rate")
	mp.GenTomRing[0] = elidedValOrNaN(elided, "tom_ring_rate")
	mp.GenTomO1[0] = elidedValOrNaN(elided, "tom_o1_gain")
	mp.GenTomO2[0] = elidedValOrNaN(elided, "tom_o2_gain")
	mp.GenTomStick[0] = elidedValOrNaN(elided, "tom_stick")
	mp.GenTomRoom[0] = elidedValOrNaN(elided, "tom_room")

	// Slots 2..12 stay at source 0 (off).
	for i := 1; i < 12; i++ {
		mp.GenSource[i] = 0
	}

	// Shared POST stage: all three tom recipes use {decay_rate=8, pitch, decay,
	// drive on; body, tone, brightness off}. postSkipped is the tom-family
	// NULL-base condition (the deleted TomParams.toC()==nil rule), inlined in
	// tomLegacyNilElision.
	postSkipped := tomLegacyNilElision(elided)
	applyFamilyPostStage(&mp, elided, postSkipped, tomPostConfig)

	return mp
}

func init() {
	// Rebind every migrated tom recipe onto the modular engine. Guarded by
	// registry membership (the tag-neutral modularMigrations registry, populated
	// by registerTomMigrations's package-var init before this init runs).
	for recipeID, spec := range tomVariantSpecs {
		if !IsModularMigratedRecipe(recipeID) {
			continue
		}
		recipeID := recipeID
		variant := spec.variant
		def := spec.def
		builtinFamilyRenderers[recipeID] = func(buf []float32, sampleRate, samples int, p RecipeParams) {
			renderModularP(buf, sampleRate, samples, tomRecipeToModular(recipeID, p, variant, def))
		}
	}
}

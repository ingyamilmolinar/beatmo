//go:build !test && !js

package audio

// snare_modular_binding.go is the native (CGo) render binding for the migrated
// snare family. Each snare recipe maps its legacy-named RecipeParams onto the
// modular engine's wide ModularParams block via the source==7 snare-ish voice
// (snare/rimshot/sidestick) or the source==8 clap voice; init() rebinds
// builtinFamilyRenderers[id] so NewRecipe(id).Render flows through
// render_modular_p. The oracle fixtures prove byte-identity.
//
// Byte-identity strategy mirrors kickRecipeToModular: the source==7/8 voices read
// each curated snare knob via kp_get(field, legacy_literal) in C, so passing NaN
// reproduces the exact legacy double literal at default while a non-default value
// arrives float32-quantized — matching the legacy NaN-elision path. The
// per-variant kp_get literals live INSIDE the C voice (one per variant branch),
// so the binding passes NaN for every curated knob the variant wires and the C
// branch's literal supplies the value. The generic wired knobs feed the shared
// POST stage exactly like the legacy render_snare_p / render_snare_rimshot_p /
// render_snare_sidestick_p / render_clap_p — including the base snare's
// drive-before-tone op order, reproduced by post_order=2.

// snareRecipeToModular maps a snare recipe's merged RecipeParams onto
// ModularParams. variant is the C discriminator (0=snare 1=rimshot 2=sidestick
// for source==7; 3=clap → source==8); def is the per-variant fundamental default.
// The wired set tells which curated knobs the variant synthesizes — the binding
// passes elidedValOrNaN for those (so a default knob is NaN → the C kp_get
// literal) and leaves the rest at the schema identity (unread for that variant).
func snareRecipeToModular(recipeID string, merged RecipeParams, variant float64, def float64, wired map[string]bool) ModularParams {
	elided := elideRecipeDefaults(recipeID, merged)

	mp := modularStageBase(recipeID, merged)

	// Voice frequency = the resolved snare fundamental (exact Hz). The source==7
	// voice reads it through gen_freq×voice_freq (ratio mode); the clap voice
	// (source==8) ignores it (pure noise).
	mp.VoiceFreqHz = snareFundamental(merged, def)

	if variant == 3 {
		mp.GenSource[0] = 8 // clap voice
	} else {
		mp.GenSource[0] = 7 // snare-ish voice
		mp.GenSnareVariant[0] = variant
	}
	mp.GenFreqMode[0] = 0 // ratio
	mp.GenFreq[0] = 1     // ×voice_freq
	// snare_wave reuses gen_wave (elidedValOrNaN → C kp_get fallback 0.0); only
	// the variants that read a tone osc wire it.
	if wired["snare_wave"] {
		mp.GenWave[0] = elidedValOrNaN(elided, "snare_wave")
	}

	// Curated knobs: drive each wired field from the elided params via
	// elidedValOrNaN (default → absent → NaN → the C kp_get per-variant literal).
	set := func(dst *float64, legacyName string) {
		if wired[legacyName] {
			*dst = elidedValOrNaN(elided, legacyName)
		}
	}
	set(&mp.GenSnareTone2[0], "snare_tone2_freq")
	set(&mp.GenSnareTune[0], "snare_noise_tune")
	set(&mp.GenSnareToneD[0], "snare_tone_decay")
	set(&mp.GenSnareNoiseD[0], "snare_noise_decay")
	set(&mp.GenSnareTailD[0], "snare_tail_decay")
	set(&mp.GenSnareToneM[0], "snare_tone_mix")
	set(&mp.GenSnareNoiseM[0], "snare_noise_mix")
	set(&mp.GenSnareWireM[0], "snare_wire_mix")
	set(&mp.GenSnareAttack[0], "snare_attack")

	// Slots 2..12 stay at source 0 (off).
	for i := 1; i < 12; i++ {
		mp.GenSource[i] = 0
	}

	// Shared POST stage: per-variant wiring (snare uses drive-before-tone via
	// post_order=2). postSkipped is the snare-family NULL-base condition (the
	// deleted SnareParams.toC()==nil rule), inlined in snareLegacyNilElision.
	postSkipped := snareLegacyNilElision(elided)
	applyFamilyPostStage(&mp, elided, postSkipped, snarePostConfigFor(variant))

	return mp
}

// snareWiredFields returns the set of legacy curated-knob names a variant
// synthesizes (the keys of its kp_get-literal map). Built from snareVariantSpecs
// so the binding and the push share one source of truth.
func snareWiredFields(recipeID string) map[string]bool {
	spec := snareVariantSpecs[recipeID]
	out := make(map[string]bool, len(spec.lit))
	for name := range spec.lit {
		out[name] = true
	}
	return out
}

func init() {
	// Rebind every migrated snare recipe onto the modular engine. Guarded by
	// registry membership (the tag-neutral modularMigrations registry, populated
	// by registerSnareMigrations's package-var init before this init runs).
	for recipeID, spec := range snareVariantSpecs {
		if !IsModularMigratedRecipe(recipeID) {
			continue
		}
		recipeID := recipeID
		variant := spec.variant
		def := spec.def
		wired := snareWiredFields(recipeID)
		builtinFamilyRenderers[recipeID] = func(buf []float32, sampleRate, samples int, p RecipeParams) {
			renderModularP(buf, sampleRate, samples, snareRecipeToModular(recipeID, p, variant, def, wired))
		}
	}
}

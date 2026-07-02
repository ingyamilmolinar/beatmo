//go:build !test && !js

package audio

// cymbal_modular_binding.go is the native (CGo) render binding for the migrated
// cymbal family. Each cymbal recipe maps its legacy-named RecipeParams onto the
// modular engine's wide ModularParams block via the source==9 metallic voice
// (variant 0=hihat 1=open-hihat 2=cowbell 3=shaker 4=ride 5=crash); init()
// rebinds builtinFamilyRenderers[id] so NewRecipe(id).Render flows through
// render_modular_p. The oracle fixtures prove byte-identity.
//
// Byte-identity strategy mirrors snareRecipeToModular: the source==9 voice reads
// each curated cymbal knob via kp_get(field, legacy_literal) in C, so passing NaN
// reproduces the exact legacy double literal at default while a non-default value
// arrives float32-quantized — matching the legacy NaN-elision path. The
// per-variant kp_get literals live INSIDE the C voice (one per variant branch),
// so the binding passes NaN for every curated knob the variant wires and the C
// branch's literal supplies the value. The generic wired knobs feed the shared
// POST stage exactly like the legacy render_hihat_p / render_open_hihat_p /
// render_cowbell_p / render_shaker_p / render_ride_p / render_crash_p — all of
// which use PostOrder=0 (the shared apply_post_params order); the cymbals wire
// brightness (not tone), so there is no drive-before-tone order trap.

// cymbalRecipeToModular maps a cymbal recipe's merged RecipeParams onto
// ModularParams. variant is the C discriminator (0=hihat 1=open-hihat 2=cowbell
// 3=shaker 4=ride 5=crash). The wired set tells which curated knobs the variant
// synthesizes — the binding passes elidedValOrNaN for those (so a default knob is
// NaN → the C kp_get literal) and leaves the rest at the schema identity (unread
// for that variant).
func cymbalRecipeToModular(recipeID string, merged RecipeParams, variant float64, wired map[string]bool) ModularParams {
	elided := elideRecipeDefaults(recipeID, merged)

	mp := modularStageBase(recipeID, merged)

	mp.GenSource[0] = 9 // metallic cymbal voice
	mp.GenCymVariant[0] = variant
	// Cymbal partials are absolute Hz; voice_freq is unused. Leave gen freq/mode
	// at the schema identity (the source==9 voice ignores them).

	// cym_wave reuses gen_wave (elidedValOrNaN → C kp_get per-variant fallback);
	// only the pitched-partial variants read it.
	if wired["cym_wave"] {
		mp.GenWave[0] = elidedValOrNaN(elided, "cym_wave")
	}

	set := func(dst *float64, legacyName string) {
		if wired[legacyName] {
			*dst = elidedValOrNaN(elided, legacyName)
		}
	}
	set(&mp.GenCymTune[0], "cym_tune")
	set(&mp.GenCymEnvFast[0], "cym_env_fast")
	set(&mp.GenCymEnvTail[0], "cym_env_tail")
	set(&mp.GenCymToneM[0], "cym_tone_mix")
	set(&mp.GenCymNoiseM[0], "cym_noise_mix")
	set(&mp.GenCymNoiseD[0], "cym_noise_decay")

	// Slots 2..12 stay at source 0 (off).
	for i := 1; i < modularGenSlots; i++ {
		mp.GenSource[i] = 0
	}

	// Shared POST stage: per-variant wiring (all cymbals use PostOrder=0).
	// postSkipped is the cymbal-family NULL-base condition (the deleted
	// CymbalParams.toC()==nil rule), inlined in cymbalLegacyNilElision.
	postSkipped := cymbalLegacyNilElision(elided)
	applyFamilyPostStage(&mp, elided, postSkipped, cymbalPostConfigFor(variant))

	return mp
}

// cymbalWiredFields returns the set of legacy curated-knob names a variant
// synthesizes (the keys of its kp_get-literal map). Built from cymbalVariantSpecs
// so the binding and the push share one source of truth.
func cymbalWiredFields(recipeID string) map[string]bool {
	spec := cymbalVariantSpecs[recipeID]
	out := make(map[string]bool, len(spec.lit))
	for name := range spec.lit {
		out[name] = true
	}
	return out
}

func init() {
	// Rebind every migrated cymbal recipe onto the modular engine. Guarded by
	// registry membership (the tag-neutral modularMigrations registry, populated
	// by registerCymbalMigrations's package-var init before this init runs).
	for recipeID, spec := range cymbalVariantSpecs {
		if !IsModularMigratedRecipe(recipeID) {
			continue
		}
		recipeID := recipeID
		variant := spec.variant
		wired := cymbalWiredFields(recipeID)
		builtinFamilyRenderers[recipeID] = func(buf []float32, sampleRate, samples int, p RecipeParams) {
			renderModularP(buf, sampleRate, samples, cymbalRecipeToModular(recipeID, p, variant, wired))
		}
	}
}

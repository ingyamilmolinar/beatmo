//go:build !test && !js

package audio

// fm_modular_binding.go is the native (CGo) render binding for the migrated FM
// family — the LAST legacy family. Each FM recipe maps its legacy-named
// RecipeParams onto the modular engine's wide ModularParams block via the
// source==10 4-operator FM preset voice (variant 0=bass 1=bell 2=lead 3=epiano
// 4=pluck); init() rebinds builtinFamilyRenderers[id] so NewRecipe(id).Render
// flows through render_modular_p. The oracle fixtures prove byte-identity.
//
// Byte-identity strategy mirrors cymbalRecipeToModular: the source==10 voice
// reads each curated FM knob via kp_get(field, preset_literal) in C, so passing
// NaN reproduces the exact legacy double literal at default (the same overlay the
// deleted fm_apply_params performed onto the static PRESET_FM_* tables) while a
// non-default value arrives float32-quantized — matching the legacy NaN-elision
// path. The per-variant preset literals live INSIDE the C voice (one switch
// branch per variant), so the binding passes NaN for every curated knob the
// variant wires and the C branch's literal supplies the value. The generic wired
// knobs feed the shared POST stage exactly like the legacy render_fm_*_p — all of
// which used PostOrder=0 (the shared apply_post_params order); the FM voice calls
// the SAME compiled fm_render the modular osc_type==4 stage uses (untouched).

// fmRecipeToModular maps an FM recipe's merged RecipeParams onto ModularParams.
// variant is the C discriminator (0=bass 1=bell 2=lead 3=epiano 4=pluck). The
// wired set tells which curated knobs the variant exposes — the binding passes
// elidedValOrNaN for those (so a default knob is NaN → the C kp_get preset
// literal) and leaves the rest at the schema identity (unread for that variant).
func fmRecipeToModular(recipeID string, merged RecipeParams, variant float64, wired map[string]bool) ModularParams {
	elided := elideRecipeDefaults(recipeID, merged)

	mp := modularStageBase(recipeID, merged)

	mp.GenSource[0] = 10 // 4-op FM preset voice
	mp.GenFMVariant[0] = variant
	// FM base freq is an absolute knob (gen_fm_base), not the derived modular
	// voice freq; leave VoiceFreqHz at identity (the source==10 voice ignores it).

	// fm_wave reuses gen_wave (elidedValOrNaN → C kp_get fallback 0 / preset sine).
	if wired["fm_wave"] {
		mp.GenWave[0] = elidedValOrNaN(elided, "fm_wave")
	}

	set := func(dst *float64, legacyName string) {
		if wired[legacyName] {
			*dst = elidedValOrNaN(elided, legacyName)
		}
	}
	set(&mp.GenFMBase[0], "fm_base_freq")
	set(&mp.GenFMPeAmt[0], "fm_pitch_env_amount")
	set(&mp.GenFMPeDecay[0], "fm_pitch_env_decay")
	set(&mp.GenFMR1[0], "fm_op1_ratio")
	set(&mp.GenFMR2[0], "fm_op2_ratio")
	set(&mp.GenFMR3[0], "fm_op3_ratio")
	set(&mp.GenFMR4[0], "fm_op4_ratio")
	set(&mp.GenFMD1[0], "fm_op1_depth")
	set(&mp.GenFMD2[0], "fm_op2_depth")
	set(&mp.GenFMD3[0], "fm_op3_depth")
	set(&mp.GenFMD4[0], "fm_op4_depth")
	set(&mp.GenFMDec1[0], "fm_op1_decay")
	set(&mp.GenFMDec2[0], "fm_op2_decay")
	set(&mp.GenFMDec3[0], "fm_op3_decay")
	set(&mp.GenFMDec4[0], "fm_op4_decay")

	// Slots 2..12 stay at source 0 (off).
	for i := 1; i < modularGenSlots; i++ {
		mp.GenSource[i] = 0
	}

	// Shared POST stage: per-variant wiring (all FM recipes use PostOrder=0).
	// postSkipped is the FM-family NULL-base condition (the deleted
	// FMParams.toC()==nil rule), inlined in fmLegacyNilElision.
	postSkipped := fmLegacyNilElision(elided)
	applyFamilyPostStage(&mp, elided, postSkipped, fmPostConfigFor(variant))

	return mp
}

// fmWiredFields returns the set of legacy curated-knob names a variant exposes
// (the keys of its kp_get-literal map). Built from fmVariantSpecs so the binding
// and the push share one source of truth.
func fmWiredFields(recipeID string) map[string]bool {
	spec := fmVariantSpecs[recipeID]
	out := make(map[string]bool, len(spec.lit))
	for name := range spec.lit {
		out[name] = true
	}
	return out
}

func init() {
	// Rebind every migrated FM recipe onto the modular engine. Guarded by
	// registry membership (the tag-neutral modularMigrations registry, populated
	// by registerFMMigrations's package-var init before this init runs).
	for recipeID, spec := range fmVariantSpecs {
		if !IsModularMigratedRecipe(recipeID) {
			continue
		}
		recipeID := recipeID
		variant := spec.variant
		wired := fmWiredFields(recipeID)
		builtinFamilyRenderers[recipeID] = func(buf []float32, sampleRate, samples int, p RecipeParams) {
			renderModularP(buf, sampleRate, samples, fmRecipeToModular(recipeID, p, variant, wired))
		}
	}
}

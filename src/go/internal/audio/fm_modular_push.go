package audio

// fm_modular_push.go holds the BUILD-TAG-NEUTRAL FM→modular registry entries:
// the five FM-family voiceParams closures (4-operator FM preset voice source==10,
// variant 0=bass 1=bell 2=lead 3=epiano 4=pluck) and their registrations into the
// family-agnostic modularMigrations registry. The generic scaffolding (push
// translation, POST stage, elision helpers) lives in family_modular_push.go.
//
// The FM family is the LAST legacy family migrated. Like the kick/tom/snare/
// cymbal families, FM instruments keep their LEGACY-named params (fm_base_freq,
// fm_op1_ratio, …, plus the wired generic pitch/decay/tone/drive/body/brightness)
// everywhere outside the push seam; translation to the modular-named block
// happens ONLY at the platform-push seam (via ModularPushParams), so the
// browser's render_modular_p sees the same effective params the native binding
// (fm_modular_binding.go) feeds C.
//
// Why a tag-neutral copy of the mapping: see the bass_modular_push.go header.
// The push side spells every kp_get fallback literal out (NaN can't survive
// JSON / the JS block-fill); the <1-ULP float32 gap is within the xplat gate.

// fmPostConfigFor returns the per-recipe POST-stage wiring. EVERY FM _p()
// wrapper (render_fm_bass_p / bell_p / lead_p / epiano_p / pluck_p, fmsynth.c)
// called the SHARED apply_post_params — none used a bespoke inline post with a
// reordered op sequence — so EVERY FM recipe uses PostOrder=0 (the shared op
// order pitch→decay→body→brightness→tone→drive). There is no drive-before-tone
// or drive-before-body trap like the base snare/kick. The five wrappers diverge
// only in their decay_rate constant and their gate set:
//
//   - render_fm_bass_p:   {decay_rate=4, pitch, decay, drive, body, tone}
//   - render_fm_bell_p:   {decay_rate=2, pitch, decay, drive, brightness}
//   - render_fm_lead_p:   {decay_rate=6, pitch, decay, drive, tone, brightness}
//   - render_fm_epiano_p: {decay_rate=3, pitch, decay, drive, tone, brightness}
//   - render_fm_pluck_p:  {decay_rate=8, pitch, decay, drive, tone}
//
// (The |tonedrive oracle case sets tone=min<0 AND drive>0 at once for the four
// tone-wiring presets; with the shared order it pins POST stage byte-identity.
// fm-bell wires brightness not tone, so it has no |tonedrive case.)
func fmPostConfigFor(variant float64) familyPostConfig {
	switch variant {
	case 1: // bell: decay_rate 2, pitch/decay/drive/brightness
		return familyPostConfig{
			DecayRate: 2.0, Pitch: true, Decay: true, Drive: true, Brightness: true,
			Tone: false, Body: false, PostOrder: 0,
		}
	case 2: // lead: decay_rate 6, pitch/decay/drive/tone/brightness
		return familyPostConfig{
			DecayRate: 6.0, Pitch: true, Decay: true, Drive: true, Tone: true, Brightness: true,
			Body: false, PostOrder: 0,
		}
	case 3: // epiano: decay_rate 3, pitch/decay/drive/tone/brightness
		return familyPostConfig{
			DecayRate: 3.0, Pitch: true, Decay: true, Drive: true, Tone: true, Brightness: true,
			Body: false, PostOrder: 0,
		}
	case 4: // pluck: decay_rate 8, pitch/decay/drive/tone
		return familyPostConfig{
			DecayRate: 8.0, Pitch: true, Decay: true, Drive: true, Tone: true,
			Body: false, Brightness: false, PostOrder: 0,
		}
	default: // bass (variant 0): decay_rate 4, pitch/decay/drive/body/tone
		return familyPostConfig{
			DecayRate: 4.0, Pitch: true, Decay: true, Drive: true, Body: true, Tone: true,
			Brightness: false, PostOrder: 0,
		}
	}
}

// fmLegacyNilElision reproduces the deleted FMParams.toC()==nil rule used by the
// native binding to decide whether the shared POST stage runs. The legacy family
// renderer passed a NULL fm_params* to render_<fm>_p (→ apply_post_params NULL
// no-op) exactly when recipeParamsToFM(p).Base.IsDefault() && .isUnset().
//
// CRITICAL FM-family difference from the drum families: recipeParamsToFM reads
// the FM family fields from the RAW MERGED map (p["fm_base_freq"], …), NOT from
// an elided map. Every FM recipe WIRES the family knobs (fm_base_freq, the op
// ratios/decays, …), so after MergeRecipeDefaults those keys are ALWAYS present
// ⇒ FMParams.isUnset() is ALWAYS false ⇒ toC() is NEVER nil ⇒ the legacy POST
// stage ALWAYS RAN (as a no-op at generic defaults, where apply_post_params
// leaves the buffer unchanged: pitch=0, decay=1, etc.). This is unlike the drum
// families, whose family fields can be absent (a variant that doesn't synthesize
// a stage), letting their *LegacyNilElision return true at the generic default.
//
// Therefore the FM POST stage is NEVER skipped — fmLegacyNilElision is
// unconditionally false. The decay=min oracle case (decay=0, Base.IsDefault()
// true) is exactly the case that PROVES this: the legacy path still ran POST
// (isUnset==false), so the modular path must too. Pinned by the |decay=min
// fixture.
func fmLegacyNilElision(elided RecipeParams) bool {
	_ = elided
	return false
}

// fmPushVoiceParams builds the FM gen-slot voice (source==10) push closure for
// one variant. recipeID identifies the recipe (for elision); variant is the C
// discriminator (0=bass 1=bell 2=lead 3=epiano 4=pluck). Every kp_get fallback
// literal is spelled out (the per-variant legacy preset literal), so an untouched
// knob arrives as the exact browser-side value the native binding's NaN sentinel
// resolves to in C.
//
// The lit map carries the per-variant fallbacks; a name absent from lit means
// that variant does not synthesize the knob (e.g. fm-bell/fm-epiano ship with no
// pitch sweep → no fm_pitch_env_amount/decay; a 2-op preset has no op3/op4). The
// push emits only the keys the variant wires, mirroring the binding's
// elidedValOrNaN. The FM base freq is an absolute knob (gen1_fm_base), NOT the
// derived modular voice freq, so voice_freq_hz is left at identity.
func fmPushVoiceParams(recipeID string, variant float64, lit map[string]float64) func(RecipeParams) RecipeParams {
	return func(merged RecipeParams) RecipeParams {
		elided := elideRecipeDefaults(recipeID, merged)
		out := RecipeParams{}
		out["gen1_source"] = 10
		out["gen1_fm_variant"] = variant

		// fm_wave reuses gen_wave (absent → 0.0, the C kp_get fallback / preset
		// default sine). Every FM preset exposes the wave knob.
		if l, ok := lit["fm_wave"]; ok {
			out["gen1_wave"] = elidedValOr(elided, "fm_wave", l)
		}

		put := func(modularName, legacyName string) {
			if l, ok := lit[legacyName]; ok {
				out[modularName] = elidedValOr(elided, legacyName, l)
			}
		}
		put("gen1_fm_base", "fm_base_freq")
		put("gen1_fm_pe_amt", "fm_pitch_env_amount")
		put("gen1_fm_pe_decay", "fm_pitch_env_decay")
		put("gen1_fm_r1", "fm_op1_ratio")
		put("gen1_fm_r2", "fm_op2_ratio")
		put("gen1_fm_r3", "fm_op3_ratio")
		put("gen1_fm_r4", "fm_op4_ratio")
		put("gen1_fm_d1", "fm_op1_depth")
		put("gen1_fm_d2", "fm_op2_depth")
		put("gen1_fm_d3", "fm_op3_depth")
		put("gen1_fm_d4", "fm_op4_depth")
		put("gen1_fm_dec1", "fm_op1_decay")
		put("gen1_fm_dec2", "fm_op2_decay")
		put("gen1_fm_dec3", "fm_op3_decay")
		put("gen1_fm_dec4", "fm_op4_decay")
		return out
	}
}

// fmVariantSpecs maps each FM recipe to its (variant code, per-variant kp_get
// literals). The literals MUST equal the PRESET_FM_* fields the source==10 voice
// reads via kp_get in modular_stages.c (the at-default byte-parity contract — the
// same literals the deleted fm_apply_params overlaid onto the static presets). A
// legacy knob absent from a variant's lit map is one the variant doesn't expose
// (fm-bell / fm-epiano have no pitch sweep; a 2-op preset has no op3/op4).
//
// Only the knobs fmFamilyParamDefs declares for each recipe are wired (the rest
// are not user-editable, so the per-variant structural literals inside the C
// voice supply them). The recipe ParamDefs (synth_recipe_wired.go) are the single
// source of truth for which knobs exist; these literals must equal those Defaults.
var fmVariantSpecs = map[string]struct {
	variant float64
	lit     map[string]float64
}{
	"fm-bass": {0, map[string]float64{
		"fm_wave": 0.0, "fm_base_freq": 55.0,
		"fm_pitch_env_amount": 3.0, "fm_pitch_env_decay": 0.06,
		"fm_op1_ratio": 1.0, "fm_op1_decay": 0.3,
		"fm_op2_ratio": 1.0, "fm_op2_depth": 2.5, "fm_op2_decay": 0.15,
	}},
	"fm-bell": {1, map[string]float64{
		"fm_wave": 0.0, "fm_base_freq": 440.0,
		// No pitch sweep (noPitchEnv) → fm_pitch_env_* not wired.
		"fm_op1_ratio": 1.0, "fm_op1_decay": 1.5,
		"fm_op2_ratio": 3.5, "fm_op2_depth": 3.0, "fm_op2_decay": 1.2,
	}},
	"fm-lead": {2, map[string]float64{
		"fm_wave": 0.0, "fm_base_freq": 220.0,
		"fm_pitch_env_amount": 1.5, "fm_pitch_env_decay": 0.04,
		"fm_op1_ratio": 1.0, "fm_op1_decay": 0.2,
		"fm_op2_ratio": 2.0, "fm_op2_depth": 3.5, "fm_op2_decay": 0.12,
		"fm_op3_ratio": 3.0, "fm_op3_depth": 2.0, "fm_op3_decay": 0.08,
	}},
	"fm-epiano": {3, map[string]float64{
		"fm_wave": 0.0, "fm_base_freq": 261.63,
		// No pitch sweep (noPitchEnv) → fm_pitch_env_* not wired.
		"fm_op1_ratio": 1.0, "fm_op1_decay": 0.8,
		"fm_op2_ratio": 1.0, "fm_op2_depth": 2.2, "fm_op2_decay": 0.2,
		"fm_op3_ratio": 2.0, "fm_op3_decay": 0.5,
	}},
	"fm-pluck": {4, map[string]float64{
		"fm_wave": 0.0, "fm_base_freq": 196.0,
		"fm_pitch_env_amount": 2.0, "fm_pitch_env_decay": 0.03,
		"fm_op1_ratio": 1.0, "fm_op1_decay": 0.2,
		"fm_op2_ratio": 2.0, "fm_op2_depth": 4.0, "fm_op2_decay": 0.04,
	}},
}

// registerFMMigrations registers the five FM family migrations into the
// tag-neutral registry, so both native and js builds route the FM recipes
// through the modular engine. Runs as a package-level VARIABLE initialization
// (not init()) so the registry is populated BEFORE any init() — in particular
// before fm_modular_binding.go's init, which reads IsModularMigratedRecipe to
// decide whether to rebind (same ordering rationale as registerCymbalMigrations).
var _ = registerFMMigrations()

func registerFMMigrations() struct{} {
	for recipeID, spec := range fmVariantSpecs {
		spec := spec
		registerModularMigration(recipeID, modularFamilyMigration{
			voiceParams: fmPushVoiceParams(recipeID, spec.variant, spec.lit),
			post:        fmPostConfigFor(spec.variant),
			postSkipped: fmLegacyNilElision,
		})
	}
	return struct{}{}
}

package audio

// cymbal_modular_push.go holds the BUILD-TAG-NEUTRAL cymbal→modular registry
// entries: the six cymbal-family voiceParams closures (metallic voice source==9,
// variant-branched 0=hihat 1=open-hihat 2=cowbell 3=shaker 4=ride 5=crash) and
// their registrations into the family-agnostic modularMigrations registry. The
// generic scaffolding (push translation, POST stage, elision helpers) lives in
// family_modular_push.go.
//
// Like the kick/tom/snare families, cymbal instruments keep their LEGACY-named
// params (cym_tune, cym_env_fast, …, plus the wired generic pitch/decay/drive/
// brightness) everywhere outside the push seam; translation to the modular-named
// block happens ONLY at the platform-push seam (via ModularPushParams), so the
// browser's render_modular_p sees the same effective params the native binding
// (cymbal_modular_binding.go) feeds C.
//
// Why a tag-neutral copy of the mapping: see the bass_modular_push.go header.
// The push side spells every kp_get fallback literal out (NaN can't survive
// JSON / the JS block-fill); the <1-ULP float32 gap is within the xplat gate.

// cymbalPostConfigFor returns the per-recipe POST-stage wiring. All six legacy
// _p() wrappers diverge in their decay_rate but agree on the gate set and the op
// ORDER:
//
//   - render_hihat_p (hihat): bespoke INLINE post = decay(rate 20) → brightness
//     → drive (NO pitch/tone/body). decay-before-brightness-before-drive is a
//     SUBSET of the shared apply_post_params order
//     (pitch→decay→body→brightness→tone→drive), so PostOrder=0.
//   - render_open_hihat_p: apply_post_params {decay_rate=20, decay, brightness,
//     drive}. PostOrder=0 (the shared order).
//   - render_cowbell_p (cowbell): bespoke INLINE post = pitch → decay(rate 12)
//     (NO drive/tone/body/brightness). pitch-before-decay is a SUBSET of the
//     shared order, so PostOrder=0.
//   - render_shaker_p: apply_post_params {decay_rate=15, decay, brightness,
//     drive}. PostOrder=0.
//   - render_ride_p: apply_post_params {decay_rate=12, decay, brightness, drive}.
//     PostOrder=0.
//   - render_crash_p: apply_post_params {decay_rate=4, decay, brightness, drive}.
//     PostOrder=0.
//
// Because no cymbal wrapper applies drive-before-tone or drive-before-body (the
// two cases that needed PostOrder 1/2 for kick/snare), every cymbal recipe uses
// PostOrder=0 — the shared apply_post_params order. The cymbals wire BRIGHTNESS,
// not tone, so there is no tone-order trap.
func cymbalPostConfigFor(variant float64) familyPostConfig {
	switch variant {
	case 0: // hihat: decay(rate 20) → brightness → drive
		return familyPostConfig{
			DecayRate: 20.0, Decay: true, Brightness: true, Drive: true,
			Pitch: false, Tone: false, Body: false, PostOrder: 0,
		}
	case 1: // open-hihat: apply_post_params {decay 20, decay, brightness, drive}
		return familyPostConfig{
			DecayRate: 20.0, Decay: true, Brightness: true, Drive: true,
			Pitch: false, Tone: false, Body: false, PostOrder: 0,
		}
	case 2: // cowbell: pitch → decay(rate 12) (NO drive/brightness)
		return familyPostConfig{
			DecayRate: 12.0, Pitch: true, Decay: true,
			Drive: false, Brightness: false, Tone: false, Body: false, PostOrder: 0,
		}
	case 3: // shaker: apply_post_params {decay 15, decay, brightness, drive}
		return familyPostConfig{
			DecayRate: 15.0, Decay: true, Brightness: true, Drive: true,
			Pitch: false, Tone: false, Body: false, PostOrder: 0,
		}
	case 4: // ride: apply_post_params {decay 12, decay, brightness, drive}
		return familyPostConfig{
			DecayRate: 12.0, Decay: true, Brightness: true, Drive: true,
			Pitch: false, Tone: false, Body: false, PostOrder: 0,
		}
	default: // crash (variant 5): apply_post_params {decay 4, decay, brightness, drive}
		return familyPostConfig{
			DecayRate: 4.0, Decay: true, Brightness: true, Drive: true,
			Pitch: false, Tone: false, Body: false, PostOrder: 0,
		}
	}
}

// cymbalLegacyNilElision reproduces the deleted CymbalParams.toC()==nil rule used
// by the native binding to decide whether the shared POST stage runs. The legacy
// family renderer passed a NULL cymbal_params* to render_<cymbal>_p (→ no post:
// the hihat/cowbell bespoke `if (!base) return`, the open-hihat/shaker/ride/crash
// variants' apply_post_params NULL no-op) exactly when the elided generic base
// was SynthParams-default (each of pitch/decay/tone/drive/body/brightness/
// fundamental == 0 after recipeParamsToSynth, which treats Decay==0 — NOT
// Decay==1 — as default) AND every cymbal family field was unset (absent from the
// elided map).
//
// Faithful, dependency-free inline of recipeParamsToCymbal(elided).Base.IsDefault()
// && .isUnset(). The cymbal oracle fixtures prove the replacement is byte-exact.
func cymbalLegacyNilElision(elided RecipeParams) bool {
	baseDefault := elidedVal(elided, "pitch", 0) == 0 &&
		elidedVal(elided, "decay", 1) == 0 &&
		elidedVal(elided, "tone", 0) == 0 &&
		elidedVal(elided, "drive", 0) == 0 &&
		elidedVal(elided, "body", 0) == 0 &&
		elidedVal(elided, "brightness", 0) == 0 &&
		elidedVal(elided, "fundamental", 0) == 0
	if !baseDefault {
		return false
	}
	for _, name := range []string{
		"cym_tune", "cym_env_fast", "cym_env_tail",
		"cym_tone_mix", "cym_noise_mix", "cym_noise_decay", "cym_wave",
	} {
		if _, ok := elided[name]; ok {
			return false
		}
	}
	return true
}

// cymbalPushVoiceParams builds the cymbal gen-slot voice (metallic, source==9)
// push closure for one variant. recipeID identifies the recipe (for elision);
// variant is the C discriminator (0=hihat 1=open-hihat 2=cowbell 3=shaker 4=ride
// 5=crash). Every kp_get fallback literal is spelled out (the per-variant legacy C
// literal), so an untouched knob arrives as the exact browser-side value the
// native binding's NaN sentinel resolves to in C.
//
// The lit map carries the per-variant fallbacks; a name absent from lit means
// that variant does not synthesize the knob (e.g. the shaker has no
// cym_noise_decay band and no oscillator → no cym_wave). The push emits only the
// keys the variant wires, mirroring the binding's elidedValOrNaN.
func cymbalPushVoiceParams(recipeID string, variant float64, lit map[string]float64) func(RecipeParams) RecipeParams {
	return func(merged RecipeParams) RecipeParams {
		elided := elideRecipeDefaults(recipeID, merged)
		out := RecipeParams{}
		out["gen1_source"] = 9
		out["gen1_cym_variant"] = variant
		// Cymbal partials are absolute Hz; voice_freq is unused. Leave gen1_freq /
		// gen1_freq_mode at the schema identity (the source==9 voice ignores them).

		// cym_wave reuses gen_wave; only the pitched-partial variants read it
		// (present in lit).
		if l, ok := lit["cym_wave"]; ok {
			out["gen1_wave"] = elidedValOr(elided, "cym_wave", l)
		}
		put := func(modularName, legacyName string) {
			if l, ok := lit[legacyName]; ok {
				out[modularName] = elidedValOr(elided, legacyName, l)
			}
		}
		put("gen1_cym_tune", "cym_tune")
		put("gen1_cym_env_fast", "cym_env_fast")
		put("gen1_cym_env_tail", "cym_env_tail")
		put("gen1_cym_tone_m", "cym_tone_mix")
		put("gen1_cym_noise_m", "cym_noise_mix")
		put("gen1_cym_noise_d", "cym_noise_decay")
		return out
	}
}

// cymbalVariantSpecs maps each cymbal recipe to its (variant code, per-variant
// kp_get literals). The literals MUST equal the render_<cymbal>_internal kp_get
// fallbacks in the legacy drums.c (the at-default byte-parity contract). A legacy
// knob absent from a variant's lit map is one the variant doesn't synthesize
// (shaker: no cym_noise_decay, no cym_wave).
var cymbalVariantSpecs = map[string]struct {
	variant float64
	lit     map[string]float64
}{
	"drum-hihat": {0, map[string]float64{
		"cym_tune": 1.0, "cym_env_fast": 180.0, "cym_env_tail": 35.0,
		"cym_tone_mix": 0.85, "cym_noise_mix": 0.45, "cym_noise_decay": 100.0,
		"cym_wave": 2.0,
	}},
	"drum-open-hihat": {1, map[string]float64{
		"cym_tune": 1.0, "cym_env_fast": 120.0, "cym_env_tail": 22.0,
		"cym_tone_mix": 0.7, "cym_noise_mix": 0.9, "cym_noise_decay": 18.0,
		"cym_wave": 2.0,
	}},
	"drum-cowbell": {2, map[string]float64{
		"cym_tune": 1.0, "cym_env_fast": 260.0, "cym_env_tail": 9.0,
		"cym_tone_mix": 1.0, "cym_noise_mix": 0.55, "cym_noise_decay": 60.0,
		"cym_wave": 0.0,
	}},
	"drum-shaker": {3, map[string]float64{
		"cym_tune": 1.0, "cym_env_fast": 200.0, "cym_env_tail": 25.0,
		"cym_tone_mix": 0.6, "cym_noise_mix": 0.5,
		// No cym_noise_decay (the shaker has no separate noise envelope) and no
		// cym_wave (pure noise — no oscillator cluster).
	}},
	"drum-ride": {4, map[string]float64{
		"cym_tune": 1.0, "cym_env_fast": 40.0, "cym_env_tail": 8.0,
		"cym_tone_mix": 1.0, "cym_noise_mix": 0.3, "cym_noise_decay": 12.0,
		"cym_wave": 0.0,
	}},
	"drum-crash": {5, map[string]float64{
		"cym_tune": 1.0, "cym_env_fast": 10.0, "cym_env_tail": 3.0,
		"cym_tone_mix": 1.0, "cym_noise_mix": 0.35, "cym_noise_decay": 8.0,
		"cym_wave": 0.0,
	}},
}

// registerCymbalMigrations registers the six cymbal family migrations into the
// tag-neutral registry, so both native and js builds route the cymbal recipes
// through the modular engine. Runs as a package-level VARIABLE initialization
// (not init()) so the registry is populated BEFORE any init() — in particular
// before cymbal_modular_binding.go's init, which reads IsModularMigratedRecipe to
// decide whether to rebind (same ordering rationale as registerSnareMigrations).
var _ = registerCymbalMigrations()

func registerCymbalMigrations() struct{} {
	for recipeID, spec := range cymbalVariantSpecs {
		spec := spec
		registerModularMigration(recipeID, modularFamilyMigration{
			voiceParams: cymbalPushVoiceParams(recipeID, spec.variant, spec.lit),
			post:        cymbalPostConfigFor(spec.variant),
			postSkipped: cymbalLegacyNilElision,
		})
	}
	return struct{}{}
}

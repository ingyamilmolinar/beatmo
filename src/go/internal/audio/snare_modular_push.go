package audio

// snare_modular_push.go holds the BUILD-TAG-NEUTRAL snare→modular registry
// entries: the four snare-family voiceParams closures (snare-ish voice
// source==7 for snare/rimshot/sidestick, clap voice source==8) and their
// registrations into the family-agnostic modularMigrations registry. The
// generic scaffolding (push translation, POST stage, elision helpers) lives in
// family_modular_push.go.
//
// Like the kick/tom families, snare instruments keep their LEGACY-named params
// (fundamental, snare_tone2_freq, snare_wave, …) everywhere outside the push
// seam; translation to the modular-named block happens ONLY at the platform-push
// seam (via ModularPushParams), so the browser's render_modular_p sees the same
// effective params the native binding (snare_modular_binding.go) feeds C.
//
// Why a tag-neutral copy of the mapping: see the bass_modular_push.go header.
// The push side spells every kp_get fallback literal out (NaN can't survive
// JSON / the JS block-fill); the <1-ULP float32 gap is within the xplat gate.

// snarePostConfigFor returns the per-recipe POST-stage wiring. The four legacy
// _p() wrappers diverge:
//
//   - render_snare_p (BASE snare): bespoke INLINE post = pitch → decay(rate 8) →
//     drive → tone (NO body/brightness). Crucially it applies drive BEFORE tone,
//     while the shared apply_post_params applies tone BEFORE drive. This is the
//     snare post-order trap — pinned by the |tonedrive oracle case (tone=-0.5,
//     drive=0.5), the only case that sets tone<0 (legacy LP active) AND drive>0 at
//     once. The |combo case sets tone=+0.4 (positive ⇒ legacy tone inert) and so
//     CANNOT distinguish the orders. PostOrder=2 reproduces the legacy
//     drive-before-tone sequence (see modular.c post_order switch).
//   - render_snare_rimshot_p / render_snare_sidestick_p: apply_post_params with
//     {decay_rate=8, pitch, decay, drive, tone} (NO body/brightness). The shared
//     order works because only one of drive/tone interacts per per-param sweep,
//     and the |combo case is rendered against the legacy shared order too →
//     PostOrder=0.
//   - render_clap_p (clap): bespoke INLINE post = decay(rate 10) → drive (NO
//     pitch/tone/body/brightness). With only decay+drive enabled, drive-after-
//     decay equals the shared order → PostOrder=0.
func snarePostConfigFor(variant float64) familyPostConfig {
	switch variant {
	case 0: // base snare: pitch/decay/drive/tone, drive BEFORE tone (post_order=2)
		return familyPostConfig{
			DecayRate: 8.0, Pitch: true, Decay: true, Drive: true, Tone: true,
			Body: false, Brightness: false, PostOrder: 2,
		}
	case 1, 2: // rimshot / sidestick: apply_post_params {decay 8, pitch, decay, drive, tone}
		return familyPostConfig{
			DecayRate: 8.0, Pitch: true, Decay: true, Drive: true, Tone: true,
			Body: false, Brightness: false, PostOrder: 0,
		}
	default: // clap (variant 3): decay(rate 10) → drive only
		return familyPostConfig{
			DecayRate: 10.0, Decay: true, Drive: true,
			Pitch: false, Tone: false, Body: false, Brightness: false, PostOrder: 0,
		}
	}
}

// snareFundamental replicates the deleted C snare_fund(base, def): the resolved
// snare primary-tone frequency in Hz. <=0 (or absent) → the per-variant default;
// otherwise clamped to [80, 2000] (the snare clamp, distinct from kick's
// [30,200], tom's [40,400], bass's [25,250]). Tag-neutral (used by both the
// native binding and the push). Clap is pure noise (no pitched body) — it still
// carries a fundamental for ABI uniformity but the source==8 voice ignores it.
func snareFundamental(merged RecipeParams, def float64) float64 {
	f := merged["fundamental"]
	if f <= 0 {
		return def
	}
	if f < 80.0 {
		f = 80.0
	}
	if f > 2000.0 {
		f = 2000.0
	}
	return f
}

// snareLegacyNilElision reproduces the deleted SnareParams.toC()==nil rule used
// by the native binding to decide whether the shared POST stage runs. The legacy
// family renderer passed a NULL snare_params* to render_<snare>_p (→ no post: the
// base snare's `if (!base) return`, the clap's `if (!base) return`, the rimshot/
// sidestick variants' apply_post_params NULL no-op) exactly when the elided
// generic base was SynthParams-default (each of pitch/decay/tone/drive/body/
// brightness/fundamental == 0 after recipeParamsToSynth, which treats Decay==0 —
// NOT Decay==1 — as default) AND every snare family field was unset (absent from
// the elided map).
//
// Faithful, dependency-free inline of recipeParamsToSnare(elided).Base.IsDefault()
// && .isUnset(). The snare oracle fixtures prove the replacement is byte-exact.
func snareLegacyNilElision(elided RecipeParams) bool {
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
		"snare_tone2_freq", "snare_noise_tune",
		"snare_tone_decay", "snare_noise_decay", "snare_tail_decay",
		"snare_tone_mix", "snare_noise_mix", "snare_wire_mix",
		"snare_attack", "snare_wave",
	} {
		if _, ok := elided[name]; ok {
			return false
		}
	}
	return true
}

// snarePushVoiceParams builds the snare gen-slot voice push closure for one
// variant. recipeID identifies the recipe (for elision and the fundamental
// default); variant is the C discriminator (0=snare 1=rimshot 2=sidestick for
// source==7; 3=clap maps to source==8). Every kp_get fallback literal is spelled
// out (the per-variant legacy C literal), so an untouched knob arrives as the
// exact browser-side value the native binding's NaN sentinel resolves to in C.
//
// The lit map carries the per-variant fallbacks; a name absent from lit means
// that variant does not synthesize the knob (e.g. rimshot/sidestick have no wire
// band → no snare_wire_mix; clap has no pitched body → no snare_tone2_freq /
// snare_tone_decay / snare_tone_mix / snare_wire_mix / snare_wave). The push
// emits only the keys the variant wires, mirroring the binding's elidedValOrNaN.
func snarePushVoiceParams(recipeID string, variant float64, def float64, lit map[string]float64) func(RecipeParams) RecipeParams {
	return func(merged RecipeParams) RecipeParams {
		elided := elideRecipeDefaults(recipeID, merged)
		out := RecipeParams{}
		out["voice_freq_hz"] = snareFundamental(merged, def)
		// Clap (variant 3) → source 8; the snare-ish trio → source 7.
		if variant == 3 {
			out["gen1_source"] = 8
		} else {
			out["gen1_source"] = 7
			out["gen1_snare_variant"] = variant
		}
		out["gen1_freq_mode"] = 0
		out["gen1_freq"] = 1
		// snare_wave reuses gen_wave (absent → 0.0, the C kp_get fallback); only
		// the variants that read a tone osc wire it.
		if _, ok := lit["snare_wave"]; ok {
			out["gen1_wave"] = elidedValOr(elided, "snare_wave", 0)
		}
		// Curated knobs: emit only those the variant wires (present in lit).
		put := func(modularName, legacyName string) {
			if l, ok := lit[legacyName]; ok {
				out[modularName] = elidedValOr(elided, legacyName, l)
			}
		}
		put("gen1_snare_tone2", "snare_tone2_freq")
		put("gen1_snare_tune", "snare_noise_tune")
		put("gen1_snare_tone_d", "snare_tone_decay")
		put("gen1_snare_noise_d", "snare_noise_decay")
		put("gen1_snare_tail_d", "snare_tail_decay")
		put("gen1_snare_tone_m", "snare_tone_mix")
		put("gen1_snare_noise_m", "snare_noise_mix")
		put("gen1_snare_wire_m", "snare_wire_mix")
		put("gen1_snare_attack", "snare_attack")
		return out
	}
}

// snareVariantSpecs maps each snare recipe to its (variant code, fundamental
// default, per-variant kp_get literals). The literals MUST equal the
// render_snare*_internal / render_clap_internal kp_get fallbacks in the legacy
// drums.c (the at-default byte-parity contract). A legacy knob absent from a
// variant's lit map is one the variant doesn't synthesize.
var snareVariantSpecs = map[string]struct {
	variant float64
	def     float64
	lit     map[string]float64
}{
	"drum-snare": {0, 186.0, map[string]float64{
		"snare_tone2_freq": 280.0, "snare_noise_tune": 0.55,
		"snare_tone_decay": 46.0, "snare_noise_decay": 7.0, "snare_tail_decay": 10.0,
		"snare_tone_mix": 1.12, "snare_noise_mix": 0.56, "snare_wire_mix": 0.8,
		"snare_attack": 0.5, "snare_wave": 0.0,
	}},
	"drum-snare-rimshot": {1, 500.0, map[string]float64{
		"snare_tone2_freq": 1050.0, "snare_noise_tune": 1.0,
		"snare_tone_decay": 40.0, "snare_noise_decay": 200.0,
		"snare_tone_mix": 1.0, "snare_noise_mix": 0.7,
		"snare_attack": 2.0, "snare_wave": 0.0,
	}},
	"drum-snare-sidestick": {2, 500.0, map[string]float64{
		"snare_tone2_freq": 1200.0, "snare_noise_tune": 1.0,
		"snare_tone_decay": 100.0, "snare_noise_decay": 150.0,
		"snare_tone_mix": 0.5, "snare_noise_mix": 0.5,
		"snare_attack": 1.0, "snare_wave": 0.0,
	}},
	"drum-clap": {3, 500.0, map[string]float64{
		"snare_noise_tune":  1.0,
		"snare_noise_decay": 7.0, "snare_tail_decay": 4.0,
		"snare_noise_mix": 0.15,
		"snare_attack":    110.0,
	}},
}

// registerSnareMigrations registers the four snare family migrations into the
// tag-neutral registry, so both native and js builds route the snare recipes
// through the modular engine. Runs as a package-level VARIABLE initialization
// (not init()) so the registry is populated BEFORE any init() — in particular
// before snare_modular_binding.go's init, which reads IsModularMigratedRecipe to
// decide whether to rebind (same ordering rationale as registerKickMigrations).
var _ = registerSnareMigrations()

func registerSnareMigrations() struct{} {
	for recipeID, spec := range snareVariantSpecs {
		spec := spec
		registerModularMigration(recipeID, modularFamilyMigration{
			voiceParams: snarePushVoiceParams(recipeID, spec.variant, spec.def, spec.lit),
			post:        snarePostConfigFor(spec.variant),
			postSkipped: snareLegacyNilElision,
		})
	}
	return struct{}{}
}

package audio

// tom_modular_push.go holds the BUILD-TAG-NEUTRAL tom→modular registry entries:
// the three tom voiceParams closures (808-style tom voice, source==6) and their
// registrations into the family-agnostic modularMigrations registry. The generic
// scaffolding (push translation, POST stage, fundamental resolver, elision
// helpers) lives in family_modular_push.go.
//
// Like the kick family, tom instruments keep their LEGACY-named params
// (fundamental, tom_sweep_rate, tom_wave, …) everywhere outside the push seam;
// translation to the modular-named block happens ONLY at the platform-push seam
// (via ModularPushParams), so the browser's render_modular_p sees the same
// effective params the native binding (tom_modular_binding.go) feeds C.
//
// Why a tag-neutral copy of the mapping: see the bass_modular_push.go header.
// The push side spells every kp_get fallback literal out (NaN can't survive
// JSON / the JS block-fill); the <1-ULP float32 gap is within the xplat gate.

// tomPostConfig is the shared-POST-stage wiring for ALL THREE tom recipes:
// {decay_rate=8, pitch, decay, drive on; body, tone, brightness off}. Verified
// against the legacy _p() post chains:
//   - render_tom_p (base):  bespoke INLINE post = pitch → decay(rate 8) → drive
//     (NO body/tone/brightness).
//   - render_tom_high_p / render_tom_low_p: apply_post_params with
//     {decay_rate=8, pitch=1, decay=1, drive=1} (NO body/tone/brightness).
//
// Because body/tone/brightness are OFF for every variant, the shared
// apply_post_params order (pitch→decay→body→brightness→tone→drive) collapses to
// the EXACT same effective sequence as the base kick's bespoke
// pitch→decay→drive. So PostOrder=0 (the shared order) is correct for all three —
// the |combo oracle fixtures confirm it (no post-order divergence: the base
// tom's inline order is a subset that matches the shared order once the disabled
// stages are removed).
var tomPostConfig = familyPostConfig{
	DecayRate: 8.0, Pitch: true, Decay: true, Drive: true,
	Body: false, Tone: false, Brightness: false,
}

// tomFundamental replicates the deleted C tom_fund(base, def): the resolved tom
// fundamental in Hz. <=0 (or absent) → the per-variant default; otherwise clamped
// to [40, 400] (the tom clamp, distinct from kick's [30,200] and bass's
// [25,250]). Tag-neutral (used by both the native binding and the push).
func tomFundamental(merged RecipeParams, def float64) float64 {
	f := merged["fundamental"]
	if f <= 0 {
		return def
	}
	if f < 40.0 {
		f = 40.0
	}
	if f > 400.0 {
		f = 400.0
	}
	return f
}

// tomLegacyNilElision reproduces the deleted TomParams.toC()==nil rule used by
// the native binding to decide whether the shared POST stage runs. The legacy
// family renderer passed a NULL tom_params* to render_<tom>_p (→ no post: the
// base tom's `if (!base) return`, the high/low variants' apply_post_params NULL
// no-op) exactly when the elided generic base was SynthParams-default (each of
// pitch/decay/tone/drive/body/brightness/fundamental == 0 after
// recipeParamsToSynth, which treats Decay==0 — NOT Decay==1 — as default) AND
// every tom family field was unset (absent from the elided map).
//
// Faithful, dependency-free inline of recipeParamsToTom(elided).Base.IsDefault()
// && .isUnset(). The tom oracle fixtures prove the replacement is byte-exact.
func tomLegacyNilElision(elided RecipeParams) bool {
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
		"tom_sweep_rate", "tom_ring_rate",
		"tom_o1_gain", "tom_o2_gain",
		"tom_stick", "tom_room", "tom_wave",
	} {
		if _, ok := elided[name]; ok {
			return false
		}
	}
	return true
}

// tomPushVoiceParams builds the tom gen-slot voice (808-style, source==6) push
// closure for one variant. recipeID identifies the recipe (for elision and the
// fundamental default); variant is the C discriminator (0=tom 1=high 2=low).
// Every kp_get fallback literal is spelled out (the per-variant legacy C
// literal), so an untouched knob arrives as the exact browser-side value the
// native binding's NaN sentinel resolves to in C. Family fields are read from the
// elided map so a default knob falls back to its literal.
func tomPushVoiceParams(recipeID string, variant float64, def float64, lit map[string]float64) func(RecipeParams) RecipeParams {
	return func(merged RecipeParams) RecipeParams {
		elided := elideRecipeDefaults(recipeID, merged)
		out := RecipeParams{}
		out["voice_freq_hz"] = tomFundamental(merged, def)
		out["gen1_source"] = 6
		out["gen1_freq_mode"] = 0
		out["gen1_freq"] = 1
		out["gen1_tom_variant"] = variant
		// tom_wave reuses gen_wave (absent → 0.0, the C kp_get fallback).
		out["gen1_wave"] = elidedValOr(elided, "tom_wave", 0)
		put := func(modularName, legacyName string) {
			out[modularName] = elidedValOr(elided, legacyName, lit[legacyName])
		}
		put("gen1_tom_sweep", "tom_sweep_rate")
		put("gen1_tom_ring", "tom_ring_rate")
		put("gen1_tom_o1", "tom_o1_gain")
		put("gen1_tom_o2", "tom_o2_gain")
		put("gen1_tom_stick", "tom_stick")
		put("gen1_tom_room", "tom_room")
		return out
	}
}

// tomVariantSpecs maps each tom recipe to its (variant code, fundamental
// default, per-variant kp_get literals). The literals MUST equal the
// render_tom*_internal kp_get fallbacks in the legacy drums.c (the at-default
// byte-parity contract).
var tomVariantSpecs = map[string]struct {
	variant float64
	def     float64
	lit     map[string]float64
}{
	"drum-tom": {0, 150.0, map[string]float64{
		"tom_sweep_rate": 18.0, "tom_ring_rate": 2.8,
		"tom_o1_gain": 0.5, "tom_o2_gain": 0.25,
		"tom_stick": 0.35, "tom_room": 0.08,
	}},
	"drum-tom-high": {1, 170.0, map[string]float64{
		"tom_sweep_rate": 22.0, "tom_ring_rate": 3.5,
		"tom_o1_gain": 0.55, "tom_o2_gain": 0.28,
		"tom_stick": 0.38, "tom_room": 0.06,
	}},
	"drum-tom-low": {2, 90.0, map[string]float64{
		"tom_sweep_rate": 14.0, "tom_ring_rate": 2.2,
		"tom_o1_gain": 0.45, "tom_o2_gain": 0.22,
		"tom_stick": 0.32, "tom_room": 0.10,
	}},
}

// registerTomMigrations registers the three tom family migrations into the
// tag-neutral registry, so both native and js builds route the tom recipes
// through the modular engine. Runs as a package-level VARIABLE initialization
// (not init()) so the registry is populated BEFORE any init() — in particular
// before tom_modular_binding.go's init, which reads IsModularMigratedRecipe to
// decide whether to rebind (same ordering rationale as registerKickMigrations).
var _ = registerTomMigrations()

func registerTomMigrations() struct{} {
	for recipeID, spec := range tomVariantSpecs {
		spec := spec
		registerModularMigration(recipeID, modularFamilyMigration{
			voiceParams: tomPushVoiceParams(recipeID, spec.variant, spec.def, spec.lit),
			post:        tomPostConfig,
			postSkipped: tomLegacyNilElision,
		})
	}
	return struct{}{}
}

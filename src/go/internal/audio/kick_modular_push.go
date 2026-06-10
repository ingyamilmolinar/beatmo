package audio

// kick_modular_push.go holds the BUILD-TAG-NEUTRAL kick→modular registry
// entries: the five kick voiceParams closures (harmonic-bank kick voice,
// source==5) and their registrations into the family-agnostic
// modularMigrations registry. The generic scaffolding (push translation, POST
// stage, fundamental resolver, elision helpers) lives in family_modular_push.go.
//
// Like the bass family, kick instruments keep their LEGACY-named params
// (fundamental, kick_h2_gain, kick_wave, …) everywhere outside the push seam;
// translation to the modular-named block happens ONLY at the platform-push seam
// (via ModularPushParams), so the browser's render_modular_p sees the same
// effective params the native binding (kick_modular_binding.go) feeds C.
//
// Why a tag-neutral copy of the mapping: see the bass_modular_push.go header.
// The push side spells every kp_get fallback literal out (NaN can't survive
// JSON / the JS block-fill); the <1-ULP float32 gap is within the xplat gate.

// kickPostConfig is the shared-POST-stage wiring for ALL FIVE kick recipes:
// {decay_rate=6, pitch, decay, body, drive on; tone, brightness off}. Verified
// against the legacy _p() post (render_kick_p's bespoke inline post + the
// deep/punchy/lofi/tight apply_post_params post_config, both
// {decay_rate=6,pitch,decay,drive,body}).
//
// POST-OP ORDER divergence (the gap the combo oracle exposed): the base kick's
// bespoke inline post (render_kick_p) applies drive BEFORE body, while
// apply_post_params (used by deep/punchy/lofi/tight) applies body before drive.
// Per-param oracle sweeps never set drive AND body non-default at once, so they
// could not tell the orders apart — but the |combo case does. kickPostConfigFor
// therefore selects PostOrder=1 (the base-kick drive-before-body sequence) for
// drum-kick and PostOrder=0 (the shared order) for the four variants.
var kickPostConfig = familyPostConfig{
	DecayRate: 6.0, Pitch: true, Decay: true, Body: true, Drive: true,
	Brightness: false, Tone: false,
}

// kickPostConfigFor returns the per-recipe POST config: identical wiring for all
// five kicks, but PostOrder=1 for the base kick (drum-kick / variant 0), whose
// legacy render_kick_p applied drive BEFORE body. The four variants used
// apply_post_params (body before drive) → PostOrder=0.
func kickPostConfigFor(variant float64) familyPostConfig {
	cfg := kickPostConfig
	if variant == 0 {
		cfg.PostOrder = 1
	}
	return cfg
}

// kickFundamental replicates the deleted C kick_fund(base, def): the resolved
// kick fundamental in Hz. <=0 (or absent) → the per-variant default; otherwise
// clamped to [30, 200] (the kick clamp, distinct from bass's [25,250]).
// Tag-neutral (used by both the native binding and the push).
func kickFundamental(merged RecipeParams, def float64) float64 {
	f := merged["fundamental"]
	if f <= 0 {
		return def
	}
	if f < 30.0 {
		f = 30.0
	}
	if f > 200.0 {
		f = 200.0
	}
	return f
}

// kickLegacyNilElision reproduces the deleted KickParams.toC()==nil rule used by
// the native binding to decide whether the shared POST stage runs. The legacy
// family renderer passed a NULL kick_params* to render_<kick>_p (→ no post: the
// base kick's `if (!base) return`, the variants' apply_post_params NULL no-op)
// exactly when the elided generic base was SynthParams-default (each of
// pitch/decay/tone/drive/body/brightness/fundamental == 0 after
// recipeParamsToSynth, which treats Decay==0 — NOT Decay==1 — as default) AND
// every kick family field was unset (absent from the elided map).
//
// Faithful, dependency-free inline of recipeParamsToKick(elided).Base.IsDefault()
// && .isUnset(). The kick oracle fixtures prove the replacement is byte-exact.
// (recipeParamsToSynth substitutes Decay's identity 1 for an absent decay key,
// so IsDefault's Decay==0 check is false at recipe defaults — the POST stage
// RUNS unless decay is explicitly dialed to 0.)
func kickLegacyNilElision(elided RecipeParams) bool {
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
		"kick_h2_gain", "kick_h3_gain", "kick_h4_gain",
		"kick_env0_rate", "kick_env1_rate",
		"kick_pitch_env_amount", "kick_pitch_env_rate",
		"kick_click", "kick_noise", "kick_wave",
	} {
		if _, ok := elided[name]; ok {
			return false
		}
	}
	return true
}

// kickPushVoiceParams builds the kick gen-slot voice (harmonic-bank, source==5)
// push closure for one variant. recipeID identifies the recipe (for elision and
// the fundamental default); variant is the C discriminator (0=base 1=deep
// 2=punchy 3=lofi 4=tight). Every kp_get fallback literal is spelled out (the
// per-variant legacy C literal), so an untouched knob arrives as the exact
// browser-side value the native binding's NaN sentinel resolves to in C. Family
// fields are read from the elided map so a default knob falls back to its literal.
//
// The lit map carries the per-variant fallbacks; a name absent from lit means
// that variant does not synthesize the knob (e.g. punchy has no thud → no
// kick_noise; lofi has no click → no kick_click). The push emits only the keys
// the variant wires, mirroring the binding's elidedValOrNaN(absent → NaN → the
// C kp_get literal); for an absent-from-lit field the push leaves the modular
// schema identity (0), which the source==5 voice never reads for that variant.
func kickPushVoiceParams(recipeID string, variant float64, def float64, lit map[string]float64) func(RecipeParams) RecipeParams {
	return func(merged RecipeParams) RecipeParams {
		elided := elideRecipeDefaults(recipeID, merged)
		out := RecipeParams{}
		out["voice_freq_hz"] = kickFundamental(merged, def)
		out["gen1_source"] = 5
		out["gen1_freq_mode"] = 0
		out["gen1_freq"] = 1
		out["gen1_kick_variant"] = variant
		out["gen1_wave"] = elidedValOr(elided, "kick_wave", 0)
		// Curated knobs: emit only those the variant wires (present in lit).
		put := func(modularName, legacyName string) {
			if l, ok := lit[legacyName]; ok {
				out[modularName] = elidedValOr(elided, legacyName, l)
			}
		}
		put("gen1_kick_h2", "kick_h2_gain")
		put("gen1_kick_h3", "kick_h3_gain")
		put("gen1_kick_h4", "kick_h4_gain")
		put("gen1_kick_env0", "kick_env0_rate")
		put("gen1_kick_env1", "kick_env1_rate")
		put("gen1_kick_pe_amt", "kick_pitch_env_amount")
		put("gen1_kick_pe_rate", "kick_pitch_env_rate")
		put("gen1_kick_click", "kick_click")
		put("gen1_kick_noise", "kick_noise")
		return out
	}
}

// kickVariantSpecs maps each kick recipe to its (variant code, fundamental
// default, per-variant kp_get literals). The literals MUST equal the
// render_kick_*_internal kp_get fallbacks in src/c/drums.c (the at-default
// byte-parity contract). A legacy knob absent from a variant's lit map is one
// the variant doesn't synthesize (so the variant's source==5 branch never reads
// the field).
var kickVariantSpecs = map[string]struct {
	variant float64
	def     float64
	lit     map[string]float64
}{
	"drum-kick": {0, 55.0, map[string]float64{
		"kick_h2_gain": 0.40, "kick_h3_gain": 0.20, "kick_h4_gain": 0.12,
		"kick_env0_rate": 5.5, "kick_env1_rate": 9.0,
		"kick_pitch_env_amount": 0.10, "kick_pitch_env_rate": 30.0,
		"kick_click": 0.35, "kick_noise": 0.18,
	}},
	"drum-kick-deep": {1, 42.0, map[string]float64{
		"kick_h2_gain": 0.15, "kick_h3_gain": 0.10,
		"kick_env0_rate": 3.5, "kick_env1_rate": 7.0,
		"kick_pitch_env_amount": 0.15, "kick_pitch_env_rate": 15.0,
		"kick_click": 0.20, "kick_noise": 0.10,
	}},
	"drum-kick-punchy": {2, 62.0, map[string]float64{
		"kick_h2_gain":          0.40,
		"kick_env0_rate":        8.0,
		"kick_env1_rate":        14.0,
		"kick_pitch_env_amount": 0.25, "kick_pitch_env_rate": 55.0,
		"kick_click": 0.45,
	}},
	"drum-kick-lofi": {3, 50.0, map[string]float64{
		"kick_h2_gain": 0.40, "kick_h3_gain": 0.20,
		"kick_env0_rate": 4.5, "kick_env1_rate": 7.0,
		"kick_pitch_env_amount": 0.08, "kick_pitch_env_rate": 20.0,
		"kick_noise": 0.25,
	}},
	"drum-kick-tight": {4, 58.0, map[string]float64{
		"kick_h2_gain": 0.35, "kick_h3_gain": 0.15,
		"kick_env0_rate": 7.5, "kick_env1_rate": 12.0,
		"kick_pitch_env_amount": 0.05, "kick_pitch_env_rate": 65.0,
		"kick_click": 0.40, "kick_noise": 0.15,
	}},
}

// registerKickMigrations registers the five kick family migrations into the
// tag-neutral registry, so both native and js builds route the kick recipes
// through the modular engine. Runs as a package-level VARIABLE initialization
// (not init()) so the registry is populated BEFORE any init() — in particular
// before kick_modular_binding.go's init, which reads IsModularMigratedRecipe to
// decide whether to rebind (same ordering rationale as registerBassMigrations).
var _ = registerKickMigrations()

func registerKickMigrations() struct{} {
	for recipeID, spec := range kickVariantSpecs {
		spec := spec
		registerModularMigration(recipeID, modularFamilyMigration{
			voiceParams: kickPushVoiceParams(recipeID, spec.variant, spec.def, spec.lit),
			post:        kickPostConfigFor(spec.variant),
			postSkipped: kickLegacyNilElision,
		})
	}
	return struct{}{}
}

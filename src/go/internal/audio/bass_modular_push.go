package audio

import "math"

// bass_modular_push.go holds the BUILD-TAG-NEUTRAL bass→modular registry
// entries: the two bass voiceParams closures (sub-bass analytic dual-osc;
// bass-guitar Karplus-String) and their registrations into the family-agnostic
// modularMigrations registry. The generic scaffolding (push translation, POST
// stage, fundamental resolver, elision helpers) lives in family_modular_push.go.
//
// Everywhere outside the push seam — prefs, export JSON, the Synth-tab UI, the
// per-instrument params manager — bass instruments keep their LEGACY-named
// params (fundamental, bass_sustain, bass_wave, …). Translation to the
// modular-named block happens ONLY at the platform-push seam (via
// ModularPushParams), so the browser's render_modular_p sees the same effective
// params the native binding (bassRecipeToModular)
// feeds the C engine.
//
// Why a tag-neutral copy of the mapping: the native binding
// (bass_modular_binding.go, //go:build !test && !js) produces a ModularParams
// struct full of NaN sentinels (the C kp_get fallback recovers the exact double
// literal). The WASM push runs in Go-WASM (//go:build js), where neither
// ModularParams nor the binding exist, and where NaN cannot survive JSON or the
// JS block-fill (NaN → JS PARAM_IDENTITY, which for the gen-slot fields is 0,
// NOT the kp_get literal). So the push MUST emit FULLY RESOLVED modular-named
// values (the kp_get literals spelled out), keyed by the modular schema names.
// The float32-quantized literals differ from the native path's exact doubles by
// <1 ULP, which the xplat correlation gate tolerates.

// subBassPostConfig / bassGuitarPostConfig are the shared-POST-stage wiring for
// the two migrated bass recipes — the single source of truth consumed by BOTH
// the native binding (bass_modular_binding.go) and the WASM push (via the
// registry), and verified by the wired-discipline test (which used to read the
// deleted C post_config).
var (
	subBassPostConfig = familyPostConfig{
		DecayRate: 4.0, Pitch: true, Decay: true, Body: true, Drive: true,
		Brightness: false, Tone: false,
	}
)

// subBassFundamental replicates the deleted C bass_fund(base, 45.0): the
// resolved sub-bass fundamental in Hz. <=0 (or absent) → default 45; otherwise
// clamped to [25, 250]. Tag-neutral (used by both the native binding and the
// push).
func subBassFundamental(merged RecipeParams) float64 {
	return fundamentalResolve(merged, 45.0)
}

// bassLegacyNilElision reproduces the deleted BassParams.toC()==nil rule used by
// the native binding to decide whether the shared POST stage runs. The legacy
// family renderer passed a NULL base to render_<bass>_p (→ apply_post_params
// no-ops) exactly when the generic base was SynthParams-default (each of
// pitch/decay/tone/drive/body/brightness/fundamental == 0 after
// recipeParamsToSynth, which treats Decay==0 — NOT Decay==1 — as default) AND
// every bass family field was unset (absent from the elided map).
//
// This is a faithful, dependency-free inline of recipeParamsToBass(elided).Base.
// IsDefault() && .isUnset() — the 42 oracle fixtures prove the replacement is
// byte-exact. (recipeParamsToSynth substitutes Decay's identity 1 for an absent
// decay key, so IsDefault's Decay==0 check is false at recipe defaults — the
// POST stage RUNS unless decay is explicitly dialed to 0.)
func bassLegacyNilElision(elided RecipeParams) bool {
	// Base IsDefault: every generic base field resolves (via recipeParamsToSynth
	// identities) to 0. pitch/tone/drive/body/brightness/fundamental identity 0;
	// decay identity 1. IsDefault requires each == 0.
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
	// isUnset: every bass family field absent from the elided map.
	for _, name := range []string{
		"bass_sustain", "bass_pluck", "bass_attack", "bass_env_rate",
		"bass_harmonic", "bass_pitch_env", "bass_wave",
	} {
		if _, ok := elided[name]; ok {
			return false
		}
	}
	return true
}

// subBassPushVoiceParams emits the sub-bass gen-slot voice (analytic dual-osc,
// source==4) plus the voice_freq_hz override and the global stage toggles. Every
// kp_get fallback literal is spelled out. Input is the MERGED legacy-named
// params; family fields are read from the elided map so an untouched knob falls
// back to the C kp_get literal. (Distinct from bass_modular_native.go's
// subBassVoiceParams, which is the baked-ModularParams native fast path.)
func subBassPushVoiceParams(merged RecipeParams) RecipeParams {
	elided := elideRecipeDefaults("drum-sub-bass", merged)
	out := RecipeParams{}
	out["voice_freq_hz"] = subBassFundamental(merged)
	// Slot 1 = analytic dual-osc voice (source==4), ratio ×1.
	out["gen1_source"] = 4
	out["gen1_freq_mode"] = 0
	out["gen1_freq"] = 1
	out["gen1_phase_mode"] = 1
	out["gen1_phase"] = math.Pi * 0.5 // kp_get fallback M_PI*0.5
	out["gen1_wave"] = elidedValOr(elided, "bass_wave", 0)
	out["gen1_harm_mix"] = elidedValOr(elided, "bass_harmonic", 0.08)
	out["gen1_pitch_env_amt"] = elidedValOr(elided, "bass_pitch_env", 0.15)
	out["gen1_pitch_env_rate"] = 40.0
	out["gen1_atk_amt"] = elidedValOr(elided, "bass_attack", 0.2)
	out["gen1_atk_rate"] = 60.0
	out["gen1_env_fast_rate"] = elidedValOr(elided, "bass_env_rate", 2.0)
	out["gen1_sat_k"] = 1.2
	out["gen1_out_scale"] = 0.95
	return out
}

// registerBassMigrations registers the sub-bass family migration into the
// tag-neutral registry, so both native and js builds route drum-sub-bass
// through the modular engine. The default fundamental (45) is carried inside
// the voiceParams closure via fundamentalResolve.
//
// This runs as a package-level VARIABLE initialization (not init()) so the
// registry is populated BEFORE any init() function — in particular before
// bass_modular_binding.go's init, which reads IsModularMigratedRecipe to decide
// whether to rebind. Go runs init() funcs in filename order; bass_modular_binding
// (b…binding) sorts before bass_modular_push (b…push), so an init()-based
// registration here would run too late. Package-level var init always precedes
// all init() funcs, eliminating the ordering hazard.
var _ = registerBassMigrations()

func registerBassMigrations() struct{} {
	registerModularMigration("drum-sub-bass", modularFamilyMigration{
		voiceParams: subBassPushVoiceParams,
		post:        subBassPostConfig,
		postSkipped: bassLegacyNilElision,
	})
	return struct{}{}
}

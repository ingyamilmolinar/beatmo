//go:build !test && !js

package audio

import (
	"testing"
)

// Family-knob contract tests for the native-engine parameterization
// migration. For every family knob exposed on a bespoke recipe:
//
//  1. Schema: the knob appears in WiredParamsForRecipe(recipe) with its
//     Default equal to the C engine's hardcoded constant (so an unedited
//     instrument reproduces the native sound).
//  2. At-default: rendering through the SynthRecipe path with the FULL
//     merged ParamDef defaults is byte-identical to the unparameterized
//     render_X (the same contract TestModularIdentityGolden locks for the
//     modular voice).
//  3. Off-default: moving the knob away from its default changes the
//     output hash (no silent no-op knobs — the top-cited synth-tab UX bug).
//
// Knob tables are explicit (wished-for API first, TDD) and double as the
// curated-set documentation for each family.

// familyKnobCase describes one curated knob on one recipe.
type familyKnobCase struct {
	name string       // ParamDef name
	def  float64      // expected Default (== C hardcoded constant)
	off  float64      // off-default test value (within Min..Max)
	aux  RecipeParams // extra overrides needed for the knob to be audible
}

// fmFamilyKnobs is the curated FM knob set per recipe. Defaults mirror the
// PRESET_FM_* literals in src/c/fmsynth.c.
var fmFamilyKnobs = map[string][]familyKnobCase{
	"fm-bass": {
		{name: "fm_wave", def: 0, off: 2},
		{name: "fm_base_freq", def: 55, off: 110},
		{name: "fm_op1_ratio", def: 1, off: 2},
		{name: "fm_op2_ratio", def: 1, off: 3.5},
		{name: "fm_op2_depth", def: 2.5, off: 5},
		{name: "fm_op1_decay", def: 0.3, off: 0.8},
		{name: "fm_op2_decay", def: 0.15, off: 0.5},
		{name: "fm_pitch_env_amount", def: 3, off: 12},
		{name: "fm_pitch_env_decay", def: 0.06, off: 0.2},
	},
	"fm-bell": {
		{name: "fm_wave", def: 0, off: 2},
		{name: "fm_base_freq", def: 440, off: 220},
		{name: "fm_op1_ratio", def: 1, off: 2},
		{name: "fm_op2_ratio", def: 3.5, off: 2},
		{name: "fm_op2_depth", def: 3, off: 6},
		{name: "fm_op1_decay", def: 1.5, off: 0.4},
		{name: "fm_op2_decay", def: 1.2, off: 0.3},
		// No pitch-env knobs: the preset ships with no sweep, and each
		// sweep knob alone is inaudible (engine gates on amount AND decay),
		// which would violate the no-silent-knobs discipline.
	},
	"fm-lead": {
		{name: "fm_wave", def: 0, off: 2},
		{name: "fm_base_freq", def: 220, off: 440},
		{name: "fm_op1_ratio", def: 1, off: 0.5},
		{name: "fm_op2_ratio", def: 2, off: 3},
		{name: "fm_op3_ratio", def: 3, off: 5},
		{name: "fm_op2_depth", def: 3.5, off: 7},
		{name: "fm_op3_depth", def: 2, off: 4},
		{name: "fm_op1_decay", def: 0.2, off: 0.6},
		{name: "fm_op2_decay", def: 0.12, off: 0.4},
		{name: "fm_op3_decay", def: 0.08, off: 0.3},
		{name: "fm_pitch_env_amount", def: 1.5, off: 10},
		{name: "fm_pitch_env_decay", def: 0.04, off: 0.2},
	},
	"fm-epiano": {
		{name: "fm_wave", def: 0, off: 2},
		{name: "fm_base_freq", def: 261.63, off: 523.26},
		{name: "fm_op1_ratio", def: 1, off: 2},
		{name: "fm_op2_ratio", def: 1, off: 3},
		{name: "fm_op3_ratio", def: 2, off: 4},
		{name: "fm_op2_depth", def: 2.2, off: 5},
		{name: "fm_op1_decay", def: 0.8, off: 0.2},
		{name: "fm_op2_decay", def: 0.2, off: 0.6},
		{name: "fm_op3_decay", def: 0.5, off: 0.1},
		// No pitch-env knobs — same rationale as fm-bell.
	},
	"fm-pluck": {
		{name: "fm_wave", def: 0, off: 2},
		{name: "fm_base_freq", def: 196, off: 392},
		{name: "fm_op1_ratio", def: 1, off: 2},
		{name: "fm_op2_ratio", def: 2, off: 3.5},
		{name: "fm_op2_depth", def: 4, off: 8},
		{name: "fm_op1_decay", def: 0.2, off: 0.6},
		{name: "fm_op2_decay", def: 0.04, off: 0.3},
		{name: "fm_pitch_env_amount", def: 2, off: 12},
		{name: "fm_pitch_env_decay", def: 0.03, off: 0.2},
	},
}

// kickFamilyKnobs is the curated kick knob set per recipe. Defaults mirror
// the per-variant literals in src/c/drums.c (render_kick_internal and the
// deep/punchy/lofi/tight variants). `fundamental` is the pre-existing extra
// (synth_params.base path) — included here so the variants' new bindings are
// contract-locked alongside the family knobs.
var kickFamilyKnobs = map[string][]familyKnobCase{
	"drum-kick": {
		{name: "kick_wave", def: 0, off: 2},
		{name: "fundamental", def: 55, off: 80},
		{name: "kick_h2_gain", def: 0.40, off: 0.9},
		{name: "kick_h3_gain", def: 0.20, off: 0.7},
		{name: "kick_h4_gain", def: 0.12, off: 0.6},
		{name: "kick_env0_rate", def: 5.5, off: 12},
		{name: "kick_env1_rate", def: 9.0, off: 20},
		{name: "kick_pitch_env_amount", def: 0.10, off: 0.4},
		{name: "kick_pitch_env_rate", def: 30, off: 80},
		{name: "kick_click", def: 0.35, off: 0.9},
		{name: "kick_noise", def: 0.18, off: 0.7},
	},
	"drum-kick-deep": {
		{name: "kick_wave", def: 0, off: 1},
		{name: "fundamental", def: 42, off: 60},
		{name: "kick_h2_gain", def: 0.15, off: 0.6},
		{name: "kick_h3_gain", def: 0.10, off: 0.5},
		{name: "kick_env0_rate", def: 3.5, off: 9},
		{name: "kick_env1_rate", def: 7.0, off: 18},
		{name: "kick_pitch_env_amount", def: 0.15, off: 0.45},
		{name: "kick_pitch_env_rate", def: 15, off: 60},
		{name: "kick_click", def: 0.20, off: 0.8},
		{name: "kick_noise", def: 0.10, off: 0.6},
	},
	"drum-kick-punchy": {
		{name: "kick_wave", def: 0, off: 3},
		{name: "fundamental", def: 62, off: 90},
		{name: "kick_h2_gain", def: 0.40, off: 0.9},
		{name: "kick_env0_rate", def: 8.0, off: 16},
		{name: "kick_env1_rate", def: 14.0, off: 28},
		{name: "kick_pitch_env_amount", def: 0.25, off: 0.5},
		{name: "kick_pitch_env_rate", def: 55, off: 100},
		{name: "kick_click", def: 0.45, off: 1.0},
	},
	"drum-kick-lofi": {
		{name: "kick_wave", def: 0, off: 2},
		{name: "fundamental", def: 50, off: 70},
		{name: "kick_h2_gain", def: 0.40, off: 0.9},
		{name: "kick_h3_gain", def: 0.20, off: 0.7},
		{name: "kick_env0_rate", def: 4.5, off: 10},
		{name: "kick_env1_rate", def: 7.0, off: 18},
		{name: "kick_pitch_env_amount", def: 0.08, off: 0.4},
		{name: "kick_pitch_env_rate", def: 20, off: 70},
		{name: "kick_noise", def: 0.25, off: 0.8},
	},
	"drum-kick-tight": {
		{name: "kick_wave", def: 0, off: 1},
		{name: "fundamental", def: 58, off: 85},
		{name: "kick_h2_gain", def: 0.35, off: 0.9},
		{name: "kick_h3_gain", def: 0.15, off: 0.6},
		{name: "kick_env0_rate", def: 7.5, off: 16},
		{name: "kick_env1_rate", def: 12.0, off: 25},
		{name: "kick_pitch_env_amount", def: 0.05, off: 0.35},
		{name: "kick_pitch_env_rate", def: 65, off: 20},
		{name: "kick_click", def: 0.40, off: 1.0},
		{name: "kick_noise", def: 0.15, off: 0.7},
	},
}

func TestKickFamilyKnobContract(t *testing.T) {
	assertFamilyKnobContract(t, kickFamilyKnobs)
}

// tomFamilyKnobs — defaults mirror the literals in render_tom /
// render_tom_high / render_tom_low (src/c/drums.c). `fundamental` is the
// pre-sweep base pitch (fBase) carried in synth_params.base.
var tomFamilyKnobs = map[string][]familyKnobCase{
	"drum-tom": {
		{name: "tom_wave", def: 0, off: 2},
		{name: "fundamental", def: 150, off: 220},
		{name: "tom_sweep_rate", def: 18, off: 50},
		{name: "tom_ring_rate", def: 2.8, off: 8},
		{name: "tom_o1_gain", def: 0.5, off: 1.0},
		{name: "tom_o2_gain", def: 0.25, off: 0.8},
		{name: "tom_stick", def: 0.35, off: 0.9},
		{name: "tom_room", def: 0.08, off: 0.4},
	},
	"drum-tom-high": {
		{name: "tom_wave", def: 0, off: 1},
		{name: "fundamental", def: 170, off: 240},
		{name: "tom_sweep_rate", def: 22, off: 60},
		{name: "tom_ring_rate", def: 3.5, off: 9},
		{name: "tom_o1_gain", def: 0.55, off: 1.0},
		{name: "tom_o2_gain", def: 0.28, off: 0.8},
		{name: "tom_stick", def: 0.38, off: 0.9},
		{name: "tom_room", def: 0.06, off: 0.4},
	},
	"drum-tom-low": {
		{name: "tom_wave", def: 0, off: 3},
		{name: "fundamental", def: 90, off: 140},
		{name: "tom_sweep_rate", def: 14, off: 45},
		{name: "tom_ring_rate", def: 2.2, off: 7},
		{name: "tom_o1_gain", def: 0.45, off: 1.0},
		{name: "tom_o2_gain", def: 0.22, off: 0.8},
		{name: "tom_stick", def: 0.32, off: 0.9},
		{name: "tom_room", def: 0.10, off: 0.5},
	},
}

func TestTomFamilyKnobContract(t *testing.T) {
	assertFamilyKnobContract(t, tomFamilyKnobs)
}

// snareFamilyKnobs — defaults mirror the literals in render_snare /
// render_snare_rimshot / render_snare_sidestick / render_clap. The family
// block is a semantic union; each variant exposes only the stages it has.
var snareFamilyKnobs = map[string][]familyKnobCase{
	"drum-snare": {
		{name: "snare_wave", def: 0, off: 2},
		{name: "fundamental", def: 200, off: 320},      // fBody1
		{name: "snare_tone2_freq", def: 330, off: 520}, // fBody2
		{name: "snare_noise_tune", def: 1.0, off: 1.8}, // ×(350/1800/3200/4500)
		{name: "snare_tone_decay", def: 28, off: 70},
		{name: "snare_noise_decay", def: 12, off: 40},
		{name: "snare_tail_decay", def: 18, off: 60},
		{name: "snare_tone_mix", def: 0.40, off: 1.0},
		{name: "snare_noise_mix", def: 1.1, off: 0.3},
		{name: "snare_wire_mix", def: 0.9, off: 0.2},
		{name: "snare_attack", def: 0.3, off: 1.5},
	},
	"drum-snare-rimshot": {
		{name: "snare_wave", def: 0, off: 1},
		{name: "fundamental", def: 500, off: 800},        // f1 (partials 2-4 fixed)
		{name: "snare_tone2_freq", def: 1050, off: 1500}, // f2
		{name: "snare_noise_tune", def: 1.0, off: 1.8},   // ×(400 HP / 5000 LP)
		{name: "snare_tone_decay", def: 40, off: 100},
		{name: "snare_noise_decay", def: 200, off: 60},
		{name: "snare_tone_mix", def: 1.0, off: 0.3},
		{name: "snare_noise_mix", def: 0.7, off: 0.1},
		{name: "snare_attack", def: 2.0, off: 0.2},
	},
	"drum-snare-sidestick": {
		{name: "snare_wave", def: 0, off: 3},
		{name: "fundamental", def: 500, off: 800},        // fWood
		{name: "snare_tone2_freq", def: 1200, off: 1800}, // fRim
		{name: "snare_noise_tune", def: 1.0, off: 1.8},   // ×1500
		{name: "snare_tone_decay", def: 100, off: 40},
		{name: "snare_noise_decay", def: 150, off: 50},
		{name: "snare_tone_mix", def: 0.5, off: 1.0},
		{name: "snare_noise_mix", def: 0.5, off: 1.0},
		{name: "snare_attack", def: 1.0, off: 3.0},
	},
	"drum-clap": {
		{name: "snare_noise_tune", def: 1.0, off: 1.8}, // ×1800 bandpass
		{name: "snare_noise_decay", def: 7, off: 20},
		{name: "snare_tail_decay", def: 4, off: 12},
		{name: "snare_noise_mix", def: 0.15, off: 0.5}, // room tail level
		{name: "snare_attack", def: 110, off: 45},      // burst sharpness
	},
}

func TestSnareFamilyKnobContract(t *testing.T) {
	assertFamilyKnobContract(t, snareFamilyKnobs)
}

// cymbalFamilyKnobs — defaults mirror the literals in render_hihat /
// open_hihat / ride / crash / cowbell / shaker. cym_tune multiplies the
// partial-frequency tables (×1.0 default = bit-identical).
var cymbalFamilyKnobs = map[string][]familyKnobCase{
	"drum-hihat": {
		{name: "cym_wave", def: 2, off: 0},
		{name: "cym_tune", def: 1.0, off: 0.7},
		{name: "cym_env_fast", def: 180, off: 60},
		{name: "cym_env_tail", def: 35, off: 10},
		{name: "cym_tone_mix", def: 0.85, off: 0.2},
		{name: "cym_noise_mix", def: 0.45, off: 1.2},
		{name: "cym_noise_decay", def: 100, off: 30},
	},
	"drum-open-hihat": {
		{name: "cym_wave", def: 2, off: 3},
		{name: "cym_tune", def: 1.0, off: 0.7},
		{name: "cym_env_fast", def: 120, off: 40},
		{name: "cym_env_tail", def: 22, off: 6},
		{name: "cym_tone_mix", def: 0.7, off: 0.2},
		{name: "cym_noise_mix", def: 0.9, off: 0.2},
		{name: "cym_noise_decay", def: 18, off: 60},
	},
	"drum-ride": {
		{name: "cym_wave", def: 0, off: 2},
		{name: "cym_tune", def: 1.0, off: 0.7},
		{name: "cym_env_fast", def: 40, off: 120},
		{name: "cym_env_tail", def: 8, off: 2},
		{name: "cym_tone_mix", def: 1.0, off: 0.3},
		{name: "cym_noise_mix", def: 0.3, off: 0.9},
		{name: "cym_noise_decay", def: 12, off: 40},
	},
	"drum-crash": {
		{name: "cym_wave", def: 0, off: 1},
		{name: "cym_tune", def: 1.0, off: 0.7},
		{name: "cym_env_fast", def: 10, off: 40},
		{name: "cym_env_tail", def: 3, off: 9},
		{name: "cym_tone_mix", def: 1.0, off: 0.3},
		{name: "cym_noise_mix", def: 0.35, off: 0.9},
		{name: "cym_noise_decay", def: 8, off: 25},
	},
	"drum-cowbell": {
		{name: "cym_wave", def: 0, off: 2},
		{name: "cym_tune", def: 1.0, off: 0.7},
		{name: "cym_env_fast", def: 260, off: 80}, // impact transient
		{name: "cym_env_tail", def: 9, off: 25},   // body ring
		{name: "cym_tone_mix", def: 1.0, off: 0.3},
		{name: "cym_noise_mix", def: 0.55, off: 0.1}, // impact level
		{name: "cym_noise_decay", def: 60, off: 15},  // metal rasp decay
	},
	"drum-shaker": {
		{name: "cym_tune", def: 1.0, off: 0.7},      // ×8000 bandpass
		{name: "cym_env_fast", def: 200, off: 60},   // burst sharpness
		{name: "cym_env_tail", def: 25, off: 8},     // overall decay
		{name: "cym_tone_mix", def: 0.6, off: 0.1},  // HP component
		{name: "cym_noise_mix", def: 0.5, off: 1.2}, // bandpass shimmer
	},
}

func TestCymbalFamilyKnobContract(t *testing.T) {
	assertFamilyKnobContract(t, cymbalFamilyKnobs)
}

// bassFamilyKnobs — defaults mirror the literals in render_bass_guitar
// (Karplus-Strong) and render_sub_bass.
var bassFamilyKnobs = map[string][]familyKnobCase{
	"drum-bass-guitar": {
		{name: "fundamental", def: 55, off: 110},
		{name: "bass_sustain", def: 0.996, off: 0.95}, // KS decay factor
		{name: "bass_pluck", def: 0.35, off: 0.8},     // pluck LP alpha
		{name: "bass_attack", def: 0.25, off: 0.9},    // finger transient
		{name: "bass_env_rate", def: 1.8, off: 6},     // global env
	},
	"drum-sub-bass": {
		{name: "bass_wave", def: 0, off: 2},
		{name: "fundamental", def: 45, off: 80},
		{name: "bass_harmonic", def: 0.08, off: 0.5},  // 2nd harmonic mix
		{name: "bass_pitch_env", def: 0.15, off: 0.5}, // attack pitch punch
		{name: "bass_attack", def: 0.2, off: 1.0},     // attack boost
		{name: "bass_env_rate", def: 2.0, off: 8},     // sustain decay
	},
}

func TestBassFamilyKnobContract(t *testing.T) {
	assertFamilyKnobContract(t, bassFamilyKnobs)
}

// rawGoldenHash renders the unparameterized native renderer for a recipe
// (from the nativeGoldenCases table) and returns its hash.
func rawGoldenHash(t *testing.T, recipeID string) string {
	t.Helper()
	for _, tc := range nativeGoldenCases {
		if tc.name == recipeID {
			buf := make([]float32, nativeGoldenSamples)
			tc.render(buf, nativeGoldenSR, nativeGoldenSamples)
			return hashFloat32(buf)
		}
	}
	t.Fatalf("recipe %q not in nativeGoldenCases", recipeID)
	return ""
}

// renderRecipe renders via the production SynthRecipe path (the exact path
// tryRecipeVoice takes), with the given user overrides merged over defaults.
func renderRecipe(t *testing.T, recipeID string, overrides RecipeParams) string {
	t.Helper()
	recipe := NewRecipe(recipeID)
	if recipe == nil {
		t.Fatalf("NewRecipe(%q) returned nil", recipeID)
	}
	merged := MergeRecipeDefaults(recipeID, overrides)
	buf := make([]float32, nativeGoldenSamples)
	recipe.Render(buf, nativeGoldenSR, nativeGoldenSamples, 0, merged)
	return hashFloat32(buf)
}

// assertFamilyKnobContract runs the 3 assertions for one family table.
func assertFamilyKnobContract(t *testing.T, knobs map[string][]familyKnobCase) {
	t.Helper()
	for recipeID, cases := range knobs {
		t.Run(recipeID, func(t *testing.T) {
			golden := rawGoldenHash(t, recipeID)

			// 1. Schema: every curated knob is declared with the exact default.
			defs := map[string]ParamDef{}
			for _, d := range WiredParamsForRecipe(recipeID) {
				defs[d.Name] = d
			}
			for _, kc := range cases {
				d, ok := defs[kc.name]
				if !ok {
					t.Errorf("%s: knob %q missing from WiredParamsForRecipe", recipeID, kc.name)
					continue
				}
				if d.Default != kc.def {
					t.Errorf("%s: knob %q default = %v, want %v (must equal the C constant)", recipeID, kc.name, d.Default, kc.def)
				}
				if kc.off < d.Min || kc.off > d.Max || kc.def < d.Min || kc.def > d.Max {
					t.Errorf("%s: knob %q range [%v,%v] excludes def %v or off %v", recipeID, kc.name, d.Min, d.Max, kc.def, kc.off)
				}
			}

			// 2. At-default: recipe path with full merged defaults == raw render.
			if got := renderRecipe(t, recipeID, nil); got != golden {
				t.Errorf("%s: recipe path at ParamDef defaults diverged from native render\n  got:    %s\n  golden: %s", recipeID, got, golden)
			}

			// 3. Off-default: each knob audibly changes the output.
			for _, kc := range cases {
				overrides := RecipeParams{kc.name: kc.off}
				for k, v := range kc.aux {
					overrides[k] = v
				}
				if got := renderRecipe(t, recipeID, overrides); got == golden {
					t.Errorf("%s: knob %q at %v (aux %v) did NOT change the output — silent no-op knob", recipeID, kc.name, kc.off, kc.aux)
				}
			}
		})
	}
}

func TestFMFamilyKnobContract(t *testing.T) {
	assertFamilyKnobContract(t, fmFamilyKnobs)
}

// TestFMParamsMarshalNaNSentinel deleted with the FM-family migration (Phase-7,
// the LAST family): the FMParams struct + recipeParamsToFM were removed. The
// NaN-sentinel convention now lives in the FM modular binding
// (fmRecipeToModular's elidedValOrNaN — an absent curated knob arrives as NaN so
// the C kp_get reproduces the preset literal); it is exercised end-to-end by the
// FM oracle |default cases (fm_migration_test.go) and TestFMPresetGolden, which
// prove byte-identity to the static presets.

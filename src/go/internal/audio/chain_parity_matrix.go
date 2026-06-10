package audio

// chain_parity_matrix.go is the SINGLE SOURCE OF TRUTH for the synth-parameter
// test matrix used by BOTH:
//   - the Go export/import byte-parity suite (internal/ui/synth_roundtrip_audio_test.go), and
//   - the desktop reference generator (cmd/export_audio --matrix / renderParamCases),
//     whose output the browser cross-platform tests compare against.
//
// Keeping the matrix here (rather than duplicating the tweak list in each test)
// means desktop and browser exercise the IDENTICAL per-stage edits — the
// "reuse logic to compare on both platforms" requirement. Every modular synth
// stage gets at least one parameter flip so a stage that the export/import path
// or the cross-platform param plumbing drops shows up as a failing case.
//
// No build tag: imported by the cmd generator (!test && !js) and the ui test
// (which links real audio under !test && !js) and must also compile under the
// stub for any package that references it.

// ParityCase is one synth-parameter edit applied to an instrument: a named
// (instrument, recipe, tweak) triple. Tweak is an overlay of canonical recipe
// param names → values; every value is inside its ParamDef Min/Max so
// SetInstrumentParams/MergeRecipeDefaults neither drops nor clamps it.
type ParityCase struct {
	Name         string
	InstrumentID string
	Recipe       string
	Tweak        RecipeParams
}

// modularParityInstrument/Recipe is the synth voice that exposes EVERY stage as
// a user-editable parameter, so one instrument can cover all stages.
const (
	modularParityInstrument = "modular"
	modularParityRecipe     = "synth-modular"
)

// ParityMatrix returns the per-stage modular tweaks (one flip per stage) plus a
// combined all-stages case. Order is fixed and part of the cross-platform
// contract (the browser replays cases in this order on one module instance).
//
// The FM case sets osc_type=4 so the FM operators are on the audible path. All
// values are inside their modular ParamDef ranges (see modular_recipe.go).
func ParityMatrix() []ParityCase {
	i, r := modularParityInstrument, modularParityRecipe
	cases := []ParityCase{
		{"stage-OSC", i, r, RecipeParams{"osc_type": 1}},
		{"stage-FM", i, r, RecipeParams{"osc_type": 4, "fm_algorithm": 1, "fm_op2_ratio": 2.5}},
		{"stage-PITCH_ENV", i, r, RecipeParams{"pitchenv_enabled": 1, "pitchenv_amt": 6}},
		{"stage-ENV", i, r, RecipeParams{"amp_attack": 0.1}},
		{"stage-FILTER", i, r, RecipeParams{"filter_cutoff": 4000, "filter_type": 1}},
		{"stage-LFO", i, r, RecipeParams{"lfo_enabled": 1, "lfo_rate": 5}},
		{"stage-BURST", i, r, RecipeParams{"burst_enabled": 1, "burst1_amp": 0.8}},
		{"stage-DRIVE", i, r, RecipeParams{"drive": 0.3}},
		{"stage-POST", i, r, RecipeParams{"gain": 1.2}},
	}
	// Combined: union of every stage's tweak (osc_type=4 keeps FM audible).
	combined := RecipeParams{}
	for _, c := range cases {
		for k, v := range c.Tweak {
			combined[k] = v
		}
	}
	cases = append(cases, ParityCase{"stage-ALL", i, r, combined})
	return cases
}

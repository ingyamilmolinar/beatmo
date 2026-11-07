package audio

import (
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// One matrix for the Go export/import parity suite and the desktop reference generator the browser compares
// against, so both exercise IDENTICAL edits; every modular stage gets a flip, so a dropped stage fails a case.
// Untagged: the cmd generator, the ui test and the stub all compile it.

// Every value is inside its ParamDef range, so SetInstrumentParams neither drops nor clamps it.
type ParityCase struct {
	Name         string
	InstrumentID instrument.ID
	Tweak        RecipeParams
}

// Exposes EVERY stage as an editable param, so one instrument covers them all.
const modularParityInstrument instrument.ID = roster.Modular

// The order is part of the cross-platform contract: the browser replays cases in it on one module instance. The FM
// case sets osc_type=4 so the operators are audible.
func ParityMatrix() []ParityCase {
	i := modularParityInstrument
	cases := []ParityCase{
		{"stage-OSC", i, RecipeParams{"osc_type": 1}},
		{"stage-FM", i, RecipeParams{"osc_type": 4, "fm_algorithm": 1, "fm_op2_ratio": 2.5}},
		{"stage-PITCH_ENV", i, RecipeParams{"pitchenv_enabled": 1, "pitchenv_amt": 6}},
		{"stage-ENV", i, RecipeParams{"amp_attack": 0.1}},
		{"stage-FILTER", i, RecipeParams{"filter_cutoff": 4000, "filter_type": 1}},
		{"stage-LFO", i, RecipeParams{"lfo_enabled": 1, "lfo_rate": 5}},
		{"stage-BURST", i, RecipeParams{"burst_enabled": 1, "burst1_amp": 0.8}},
		{"stage-DRIVE", i, RecipeParams{"drive": 0.3}},
		{"stage-POST", i, RecipeParams{"gain": 1.2}},
	}
	// Combined: union of every stage's tweak (osc_type=4 keeps FM audible).
	combined := RecipeParams{}
	for _, c := range cases {
		for k, v := range c.Tweak {
			combined[k] = v
		}
	}
	cases = append(cases, ParityCase{"stage-ALL", i, combined})
	return cases
}

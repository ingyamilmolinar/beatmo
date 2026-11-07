package audio

import "testing"

// The FROZEN PREFIX of the block: the bridge fills it by index and C reads it as a struct, so it must mirror the C
// field order. The gen bank follows it (TestModularGenBankSchemaShape).
var modularSchemaWant = []string{
	"osc_type", "osc_detune", "osc_octave",
	"fm_algorithm",
	"fm_op1_ratio", "fm_op2_ratio", "fm_op3_ratio", "fm_op4_ratio",
	"fm_op1_depth", "fm_op2_depth", "fm_op3_depth", "fm_op4_depth",
	"fm_op1_level", "fm_op2_level", "fm_op3_level", "fm_op4_level",
	"amp_attack", "amp_decay", "amp_sustain", "amp_release", "amp_curve",
	"filter_type", "filter_cutoff", "filter_resonance",
	"drive", "pitch", "gain",
	"osc_enabled", "fm_enabled", "env_enabled", "filter_enabled", "drive_enabled",
	"noise_seed",
}

func TestModularParamSchemaShape(t *testing.T) {
	got := ModularParamSchema()
	if len(got) < len(modularSchemaWant) {
		t.Fatalf("modular schema len = %d, shorter than frozen prefix %d (%v)", len(got), len(modularSchemaWant), got)
	}
	for i, n := range modularSchemaWant {
		if got[i] != n {
			t.Errorf("modular schema[%d] = %q, want %q (frozen prefix moved — ABI breakage)", i, got[i], n)
		}
	}
	idx := ModularParamSchemaIndexMap()
	for i, n := range modularSchemaWant {
		if idx[n] != i {
			t.Errorf("modular index map %q = %d, want %d", n, idx[n], i)
		}
	}
	ident := modularFieldInits()
	if len(ident) != len(got) {
		t.Errorf("modular identity map has %d entries, want %d (one per schema field)", len(ident), len(got))
	}
	for _, n := range modularSchemaWant {
		if _, ok := ident[n]; !ok {
			t.Errorf("modular identity map missing %q", n)
		}
	}
}

package audio

import "testing"

// modularSchemaWant is the canonical FROZEN PREFIX of the modular voice param
// block: every pre-Phase-1 field, in ABI order. It MUST mirror the leading
// field declaration order of the C modular_params struct (src/c/modular.h) and
// the ModularParams Go struct (modular_c.go), because the WASM bridge fills
// heap[MODULAR_PARAM_INDEX[name]] and passes the block as a const
// modular_params*. The Phase-1 gen bank is APPENDED after this prefix (its
// contiguous field-major layout is asserted by TestModularGenBankSchemaShape),
// so this test now verifies the prefix is unmoved rather than the full length.
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
	ident := ModularParamSchemaIdentity()
	if len(ident) != len(got) {
		t.Errorf("modular identity map has %d entries, want %d (one per schema field)", len(ident), len(got))
	}
	for _, n := range modularSchemaWant {
		if _, ok := ident[n]; !ok {
			t.Errorf("modular identity map missing %q", n)
		}
	}
}

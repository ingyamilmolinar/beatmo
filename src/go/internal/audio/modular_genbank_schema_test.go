package audio

import (
	"fmt"
	"strings"
	"testing"
)

// Phase-1 gen-bank schema invariants
// (docs/superpowers/specs/2026-06-05-modular-synth-unification-design.md §1).
//
// The gen bank is append-only by-field arrays: for each per-slot field the
// schema lists gen1_<f>..gen12_<f> consecutively, AFTER every pre-existing
// modular param (never insert above — JS writes heap by flat index).

const genSlots = 12

// genSlotFields and genGlobalsPhase2 below DELIBERATELY restate the production
// field lists (modularGenSlotFields / modularGlobalsPhase2 in
// synth_param_schema.go) rather than importing them. This independent
// restatement is the point of the test: an accidental edit to the production
// tables (reorder, rename, identity change) diverges from this fixed copy and
// trips the invariants — importing the prod slice would make the test tautological.
var genSlotFields = []struct {
	Field    string
	Identity float64
}{
	{"source", 0}, {"wave", 0}, {"freq_mode", 0}, {"freq", 1}, {"gain", 1},
	{"env_fast_rate", 0}, {"env_tail_rate", 0}, {"env_fast_mix", 1}, {"env_tail_mix", 0},
	{"filt_type", 0}, {"filt_alpha", 0}, {"filt_freq", 1000}, {"filt_q", 0.707},
	{"phase_mode", 0}, {"phase", 0}, {"noise_offset", 0},
	// ── Phase-2 (bass family): analytic-voice per-slot columns (source==4). ──
	{"pitch_env_amt", 0}, {"pitch_env_rate", 0}, {"harm_mix", 0},
	{"atk_amt", 0}, {"atk_rate", 0}, {"sat_k", 0}, {"out_scale", 1},
}

// genGlobalsPhase2 are the Phase-2 globals appended at the very END of the
// modular schema (after every gen-bank per-slot array). APPEND-ONLY.
var genGlobalsPhase2 = []struct {
	Name     string
	Identity float64
}{
	{"voice_freq_hz", 0},
	{"post_enabled", 0},
	{"post_pitch", 0}, {"post_decay", 1}, {"post_decay_rate", 0},
	{"post_body", 0}, {"post_brightness", 0}, {"post_tone", 0}, {"post_drive", 0},
	{"post_pitch_on", 1}, {"post_decay_on", 1}, {"post_body_on", 1},
	{"post_brightness_on", 1}, {"post_tone_on", 1}, {"post_drive_on", 1},
}

// genSlotFieldsPhase2KS are the Phase-2 Task-2 Karplus-Strong (source==3)
// per-slot columns appended at the VERY tail of the modular schema (after the
// Phase-2 globals). APPEND-ONLY. Independent restatement of
// modularGenSlotFieldsPhase2KS — see the genSlotFields rationale above.
var genSlotFieldsPhase2KS = []struct {
	Field    string
	Identity float64
}{
	{"ks_sustain", 0}, {"ks_pluck", 0}, {"ks_blow", 0},
}

// genSlotFieldsPhase3Kick are the Phase-3 kick-family (source==5) per-slot
// columns appended at the NEW very tail of the modular schema (after the
// Phase-2 KS columns). APPEND-ONLY. Independent restatement of
// modularGenSlotFieldsPhase3Kick — see the genSlotFields rationale above.
var genSlotFieldsPhase3Kick = []struct {
	Field    string
	Identity float64
}{
	{"kick_variant", 0},
	{"kick_h2", 0}, {"kick_h3", 0}, {"kick_h4", 0},
	{"kick_env0", 0}, {"kick_env1", 0},
	{"kick_pe_amt", 0}, {"kick_pe_rate", 0},
	{"kick_click", 0}, {"kick_noise", 0},
}

// genGlobalsPhase3 are the Phase-3 globals (post_order) appended at the VERY END
// of the modular schema (after the Phase-3 kick per-slot columns). APPEND-ONLY.
// Independent restatement of modularGlobalsPhase3.
var genGlobalsPhase3 = []struct {
	Name     string
	Identity float64
}{
	{"post_order", 0},
}

// genSlotFieldsPhase4Tom are the Phase-4 tom-family (source==6) per-slot columns
// appended at the NEW very tail of the modular schema (after the Phase-3
// globals). APPEND-ONLY. Independent restatement of
// modularGenSlotFieldsPhase4Tom — see the genSlotFields rationale above.
var genSlotFieldsPhase4Tom = []struct {
	Field    string
	Identity float64
}{
	{"tom_variant", 0},
	{"tom_sweep", 0}, {"tom_ring", 0},
	{"tom_o1", 0}, {"tom_o2", 0},
	{"tom_stick", 0}, {"tom_room", 0},
}

// genSlotFieldsPhase5Snare are the Phase-5 snare-family (source==7 snare/rimshot/
// sidestick + source==8 clap) per-slot columns appended at the NEW very tail of
// the modular schema (after the Phase-4 tom columns). APPEND-ONLY. Independent
// restatement of modularGenSlotFieldsPhase5Snare — see the genSlotFields
// rationale above.
var genSlotFieldsPhase5Snare = []struct {
	Field    string
	Identity float64
}{
	{"snare_variant", 0},
	{"snare_tone2", 0}, {"snare_tune", 0},
	{"snare_tone_d", 0}, {"snare_noise_d", 0}, {"snare_tail_d", 0},
	{"snare_tone_m", 0}, {"snare_noise_m", 0}, {"snare_wire_m", 0},
	{"snare_attack", 0},
}

// genSlotFieldsPhase6Cymbal are the Phase-6 cymbal-family (source==9, 6 variant
// branches: 0=hihat 1=open-hihat 2=cowbell 3=shaker 4=ride 5=crash) per-slot
// columns appended at the NEW very tail of the modular schema (after the Phase-5
// snare columns). APPEND-ONLY. Independent restatement of
// modularGenSlotFieldsPhase6Cymbal — see the genSlotFields rationale above.
var genSlotFieldsPhase6Cymbal = []struct {
	Field    string
	Identity float64
}{
	{"cym_variant", 0},
	{"cym_tune", 0}, {"cym_env_fast", 0}, {"cym_env_tail", 0},
	{"cym_tone_m", 0}, {"cym_noise_m", 0}, {"cym_noise_d", 0},
}

// genSlotFieldsPhase7FM are the Phase-7 FM-family (source==10, 5 variant
// branches: 0=bass 1=bell 2=lead 3=epiano 4=pluck) per-slot columns appended at
// the NEW very tail of the modular schema (after the Phase-6 cymbal columns).
// APPEND-ONLY. Independent restatement of modularGenSlotFieldsPhase7FM — see the
// genSlotFields rationale above.
var genSlotFieldsPhase7FM = []struct {
	Field    string
	Identity float64
}{
	{"fm_variant", 0},
	{"fm_base", 0}, {"fm_pe_amt", 0}, {"fm_pe_decay", 0},
	{"fm_r1", 0}, {"fm_r2", 0}, {"fm_r3", 0}, {"fm_r4", 0},
	{"fm_d1", 0}, {"fm_d2", 0}, {"fm_d3", 0}, {"fm_d4", 0},
	{"fm_dec1", 0}, {"fm_dec2", 0}, {"fm_dec3", 0}, {"fm_dec4", 0},
}

// genGlobalsPhase8 are the Phase-8C modulator-stage globals (PITCH ENV / LFO /
// BURST — the spec-§1 gap closure) appended at the NEW very tail of the modular
// schema (after the Phase-7 FM columns). APPEND-ONLY. All identities are 0:
// each *_enabled defaults DISABLED (new stages — off is the byte-identity
// polarity) and a disabled stage never reads its numerics. Independent
// restatement of modularGlobalsPhase8 — see the genSlotFields rationale above.
var genGlobalsPhase8 = []struct {
	Name     string
	Identity float64
}{
	{"pitchenv_enabled", 0}, {"pitchenv_amt", 0}, {"pitchenv_decay", 0},
	{"lfo_enabled", 0}, {"lfo_rate", 0}, {"lfo_depth", 0},
	{"burst_enabled", 0}, {"burst_sharp", 0},
	{"burst1_off", 0}, {"burst1_amp", 0},
	{"burst2_off", 0}, {"burst2_amp", 0},
	{"burst3_off", 0}, {"burst3_amp", 0},
	{"burst4_off", 0}, {"burst4_amp", 0},
	{"lfo_target", 0},
	{"filtenv_enabled", 0}, {"filtenv_amt", 0}, {"filtenv_decay", 0}, {"filtenv_attack", 0},
	{"unison_voices", 1}, {"unison_detune", 0}, {"unison_mix", 0.5},
	{"unison_drift_rate", 0}, {"unison_drift_depth", 0},
	{"lfo_delay", 0},
	{"body_model", 0}, {"body_mix", 0}, {"bow_dynamics", 0},
}

// genSlotFieldsPhase9KickExtra mirrors modularGenSlotFieldsPhase9KickExtra: the
// kick voice's structural shaping constants (attack/fade/sat) promoted to
// per-slot knobs, appended field-major at the NEW very tail (after the Phase-8C
// globals). APPEND-ONLY. Identities 0 (read only at source==5).
var genSlotFieldsPhase9KickExtra = []struct {
	Field    string
	Identity float64
}{
	{"kick_attack", 0}, {"kick_fade", 0}, {"kick_sat", 0},
}

// genGlobalsPhase10 mirrors modularGlobalsPhase10: the KICK-stage enable global
// gating the source==5 kick voice. APPEND-ONLY at the VERY tail (after the
// Phase-9 kick-extra columns). Identity 0 (off).
var genGlobalsPhase10 = []struct {
	Name     string
	Identity float64
}{
	{"kick_enabled", 0},
}

func TestModularGenBankSchemaShape(t *testing.T) {
	schema := ModularParamSchema()
	idx := ModularParamSchemaIndexMap()
	ident := ModularParamSchemaIdentity()

	// 1. Everything pre-gen-bank keeps its index (append-only proof).
	frozenPrefix := []string{
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
	for i, name := range frozenPrefix {
		if idx[name] != i {
			t.Fatalf("pre-existing modular param %q moved: index %d, want %d (ABI breakage)", name, idx[name], i)
		}
	}

	// 2. Gen-bank fields exist, field-major, contiguous, after the prefix.
	pos := len(frozenPrefix) + 2 // + noise_draws + noise_prelude globals
	if idx["noise_draws"] != len(frozenPrefix) || idx["noise_prelude"] != len(frozenPrefix)+1 {
		t.Fatalf("noise bus globals misplaced: noise_draws=%d noise_prelude=%d, want %d/%d",
			idx["noise_draws"], idx["noise_prelude"], len(frozenPrefix), len(frozenPrefix)+1)
	}
	if ident["noise_draws"] != 1 || ident["noise_prelude"] != 0 {
		t.Fatalf("noise bus identities wrong: draws=%v prelude=%v, want 1/0", ident["noise_draws"], ident["noise_prelude"])
	}
	for _, f := range genSlotFields {
		for k := 1; k <= genSlots; k++ {
			name := fmt.Sprintf("gen%d_%s", k, f.Field)
			got, ok := idx[name]
			if !ok {
				t.Fatalf("schema missing %q", name)
			}
			if got != pos {
				t.Fatalf("%q at index %d, want %d (field-major contiguous layout)", name, got, pos)
			}
			if iv, ok := ident[name]; !ok || iv != f.Identity {
				t.Fatalf("%q identity = %v (present=%v), want %v", name, iv, ok, f.Identity)
			}
			pos++
		}
	}

	// 2b. Phase-2 globals: appended contiguously AFTER every gen-bank array.
	for _, g := range genGlobalsPhase2 {
		got, ok := idx[g.Name]
		if !ok {
			t.Fatalf("schema missing Phase-2 global %q", g.Name)
		}
		if got != pos {
			t.Fatalf("Phase-2 global %q at index %d, want %d (must append after the gen bank)", g.Name, got, pos)
		}
		if iv, ok := ident[g.Name]; !ok || iv != g.Identity {
			t.Fatalf("Phase-2 global %q identity = %v (present=%v), want %v", g.Name, iv, ok, g.Identity)
		}
		pos++
	}

	// 2c. Phase-2 Task-2 KS per-slot columns: appended field-major at the VERY
	//     tail, AFTER every Phase-2 global.
	for _, f := range genSlotFieldsPhase2KS {
		for k := 1; k <= genSlots; k++ {
			name := fmt.Sprintf("gen%d_%s", k, f.Field)
			got, ok := idx[name]
			if !ok {
				t.Fatalf("schema missing KS tail field %q", name)
			}
			if got != pos {
				t.Fatalf("KS tail field %q at index %d, want %d (must append after the Phase-2 globals)", name, got, pos)
			}
			if iv, ok := ident[name]; !ok || iv != f.Identity {
				t.Fatalf("KS tail field %q identity = %v (present=%v), want %v", name, iv, ok, f.Identity)
			}
			pos++
		}
	}

	// 2d. Phase-3 kick per-slot columns: appended field-major at the NEW very
	//     tail, AFTER every Phase-2 KS column.
	for _, f := range genSlotFieldsPhase3Kick {
		for k := 1; k <= genSlots; k++ {
			name := fmt.Sprintf("gen%d_%s", k, f.Field)
			got, ok := idx[name]
			if !ok {
				t.Fatalf("schema missing kick tail field %q", name)
			}
			if got != pos {
				t.Fatalf("kick tail field %q at index %d, want %d (must append after the Phase-2 KS columns)", name, got, pos)
			}
			if iv, ok := ident[name]; !ok || iv != f.Identity {
				t.Fatalf("kick tail field %q identity = %v (present=%v), want %v", name, iv, ok, f.Identity)
			}
			pos++
		}
	}

	// 2e. Phase-3 globals (post_order): appended at the NEW very tail, AFTER the
	//     Phase-3 kick per-slot columns.
	for _, g := range genGlobalsPhase3 {
		got, ok := idx[g.Name]
		if !ok {
			t.Fatalf("schema missing Phase-3 global %q", g.Name)
		}
		if got != pos {
			t.Fatalf("Phase-3 global %q at index %d, want %d (must append after the kick tail)", g.Name, got, pos)
		}
		if iv, ok := ident[g.Name]; !ok || iv != g.Identity {
			t.Fatalf("Phase-3 global %q identity = %v (present=%v), want %v", g.Name, iv, ok, g.Identity)
		}
		pos++
	}

	// 2f. Phase-4 tom per-slot columns: appended field-major at the NEW very
	//     tail, AFTER the Phase-3 globals (post_order).
	for _, f := range genSlotFieldsPhase4Tom {
		for k := 1; k <= genSlots; k++ {
			name := fmt.Sprintf("gen%d_%s", k, f.Field)
			got, ok := idx[name]
			if !ok {
				t.Fatalf("schema missing tom tail field %q", name)
			}
			if got != pos {
				t.Fatalf("tom tail field %q at index %d, want %d (must append after the Phase-3 globals)", name, got, pos)
			}
			if iv, ok := ident[name]; !ok || iv != f.Identity {
				t.Fatalf("tom tail field %q identity = %v (present=%v), want %v", name, iv, ok, f.Identity)
			}
			pos++
		}
	}

	// 2g. Phase-5 snare per-slot columns: appended field-major at the NEW very
	//     tail, AFTER the Phase-4 tom columns.
	for _, f := range genSlotFieldsPhase5Snare {
		for k := 1; k <= genSlots; k++ {
			name := fmt.Sprintf("gen%d_%s", k, f.Field)
			got, ok := idx[name]
			if !ok {
				t.Fatalf("schema missing snare tail field %q", name)
			}
			if got != pos {
				t.Fatalf("snare tail field %q at index %d, want %d (must append after the Phase-4 tom columns)", name, got, pos)
			}
			if iv, ok := ident[name]; !ok || iv != f.Identity {
				t.Fatalf("snare tail field %q identity = %v (present=%v), want %v", name, iv, ok, f.Identity)
			}
			pos++
		}
	}

	// 2h. Phase-6 cymbal per-slot columns: appended field-major at the NEW very
	//     tail, AFTER the Phase-5 snare columns.
	for _, f := range genSlotFieldsPhase6Cymbal {
		for k := 1; k <= genSlots; k++ {
			name := fmt.Sprintf("gen%d_%s", k, f.Field)
			got, ok := idx[name]
			if !ok {
				t.Fatalf("schema missing cymbal tail field %q", name)
			}
			if got != pos {
				t.Fatalf("cymbal tail field %q at index %d, want %d (must append after the Phase-5 snare columns)", name, got, pos)
			}
			if iv, ok := ident[name]; !ok || iv != f.Identity {
				t.Fatalf("cymbal tail field %q identity = %v (present=%v), want %v", name, iv, ok, f.Identity)
			}
			pos++
		}
	}

	// 2i. Phase-7 FM per-slot columns: appended field-major at the NEW very tail,
	//     AFTER the Phase-6 cymbal columns.
	for _, f := range genSlotFieldsPhase7FM {
		for k := 1; k <= genSlots; k++ {
			name := fmt.Sprintf("gen%d_%s", k, f.Field)
			got, ok := idx[name]
			if !ok {
				t.Fatalf("schema missing FM tail field %q", name)
			}
			if got != pos {
				t.Fatalf("FM tail field %q at index %d, want %d (must append after the Phase-6 cymbal columns)", name, got, pos)
			}
			if iv, ok := ident[name]; !ok || iv != f.Identity {
				t.Fatalf("FM tail field %q identity = %v (present=%v), want %v", name, iv, ok, f.Identity)
			}
			pos++
		}
	}

	// 2j. Phase-8C modulator-stage globals: appended at the NEW very tail, AFTER
	//     the Phase-7 FM columns.
	for _, g := range genGlobalsPhase8 {
		got, ok := idx[g.Name]
		if !ok {
			t.Fatalf("schema missing Phase-8 global %q", g.Name)
		}
		if got != pos {
			t.Fatalf("Phase-8 global %q at index %d, want %d (must append after the Phase-7 FM columns)", g.Name, got, pos)
		}
		if iv, ok := ident[g.Name]; !ok || iv != g.Identity {
			t.Fatalf("Phase-8 global %q identity = %v (present=%v), want %v", g.Name, iv, ok, g.Identity)
		}
		pos++
	}

	// 2k. Phase-9 kick-extra per-slot columns: appended field-major at the NEW
	//     very tail, AFTER the Phase-8C modulator globals.
	for _, f := range genSlotFieldsPhase9KickExtra {
		for k := 1; k <= genSlots; k++ {
			name := fmt.Sprintf("gen%d_%s", k, f.Field)
			got, ok := idx[name]
			if !ok {
				t.Fatalf("schema missing Phase-9 kick-extra field %q", name)
			}
			if got != pos {
				t.Fatalf("Phase-9 kick-extra field %q at index %d, want %d (must append after the Phase-8C globals)", name, got, pos)
			}
			if iv, ok := ident[name]; !ok || iv != f.Identity {
				t.Fatalf("Phase-9 kick-extra field %q identity = %v (present=%v), want %v", name, iv, ok, f.Identity)
			}
			pos++
		}
	}

	// 2l. Phase-10 KICK-stage enable global: appended at the NEW very tail, AFTER
	//     the Phase-9 kick-extra columns.
	for _, g := range genGlobalsPhase10 {
		got, ok := idx[g.Name]
		if !ok {
			t.Fatalf("schema missing Phase-10 global %q", g.Name)
		}
		if got != pos {
			t.Fatalf("Phase-10 global %q at index %d, want %d (must append after the Phase-9 kick-extra columns)", g.Name, got, pos)
		}
		if iv, ok := ident[g.Name]; !ok || iv != g.Identity {
			t.Fatalf("Phase-10 global %q identity = %v (present=%v), want %v", g.Name, iv, ok, g.Identity)
		}
		pos++
	}

	if len(schema) != pos {
		t.Fatalf("schema has %d entries, want exactly %d (no stray params after the Phase-10 KICK enable)", len(schema), pos)
	}

	// 3. ParamDefs stay 1:1 with the schema, and every gen-bank def is hidden
	//    until Phase 8 reveals the GEN section.
	defs := ModularSynthParamDefs()
	if len(defs) != len(schema) {
		t.Fatalf("ModularSynthParamDefs has %d defs, schema has %d — must stay 1:1", len(defs), len(schema))
	}
	for i, d := range defs {
		if d.Name != schema[i] {
			t.Fatalf("def[%d] = %q, schema[%d] = %q — order must mirror the ABI", i, d.Name, i, schema[i])
		}
		if strings.HasPrefix(d.Name, "gen") || d.Name == "noise_draws" || d.Name == "noise_prelude" {
			// The slot-1 kick knobs (gen1_kick_*) are deliberately surfaced as the
			// visible KICK Synth-tab stage (group "kick"); every other gen-bank def
			// stays hidden. Slots 2..12 remain hidden (ABI completeness only).
			surfacedKick := strings.HasPrefix(d.Name, "gen1_kick_")
			if !surfacedKick && d.Group != SynthHiddenGroup {
				t.Fatalf("%q must be Group:hidden until Phase 8 (got %q)", d.Name, d.Group)
			}
			if d.Default != ident[d.Name] {
				t.Fatalf("%q ParamDef default %v != schema identity %v", d.Name, d.Default, ident[d.Name])
			}
		}
	}
}

package audio

import (
	"fmt"
)

// SynthParamSchema is the canonical ordered list of names in the C
// synth_params struct. The order MUST match the field declaration order in
// src/c/synth_params.h — every consumer that pokes the struct memory layout
// directly (audio.js, plugin authors, golden tests) derives its index map
// from this slice instead of hardcoding positions.
//
// Phase 2 of the live-instrument synthesis remediation plan replaced the
// positional heap[0..5] write loop in audio.js with a schema-keyed indirection
// driven by SynthParamSchemaIndexMap(). The motivating bug: any future C
// struct reorder would silently swap params (kick "drive" becomes kick "tone"
// etc.) because audio.js wrote heap[3] for drive regardless of the struct
// layout. The schema makes the dependency structural and the drift test in
// synth_param_schema_test.go locks it.
//
// Adding or removing a synth_params field requires:
//
//  1. Edit src/c/synth_params.h (struct field + sp_* accessor + identity)
//  2. Edit src/go/internal/audio/synth_params.go (Go SynthParams struct +
//     ToRecipeParams + toCParams + IsDefault)
//  3. Edit THIS slice (synthParamSchema below) to mirror the new C order
//  4. Re-run go generate ./internal/audio (or the gen-synth-abi cmd) to
//     regenerate src/js/synth_param_abi.gen.js
//  5. synth_param_schema_test.go will fail if step 4 is skipped.
var synthParamSchema = []string{
	"pitch",
	"decay",
	"tone",
	"drive",
	"body",
	"brightness",
	"fundamental",
}

// SynthParamSchema returns the canonical ordered name list. Fresh copy so
// callers may sort/mutate without disturbing the package-level slice.
func SynthParamSchema() []string {
	out := make([]string, len(synthParamSchema))
	copy(out, synthParamSchema)
	return out
}

// SynthParamSchemaIndexMap returns a fresh map of paramName → struct index
// suitable for code that writes to the C struct by name (audio.js positional
// write loop replacement). Indices match the field declaration order in
// src/c/synth_params.h.
func SynthParamSchemaIndexMap() map[string]int {
	out := make(map[string]int, len(synthParamSchema))
	for i, name := range synthParamSchema {
		out[name] = i
	}
	return out
}

// SynthParamSchemaIdentity returns the identity (no-op) default for each
// canonical synth param. Used by the JS bridge to fill unset knobs without
// hardcoding the values in two languages. The identity values match the
// hardcoded defaults in audio.js getNumber() AND the C sp_* accessor
// fallbacks in synth_params.h.
//
// CRITICAL: changing an identity value here without also updating the C
// sp_* fallback produces a silent behavioral drift (e.g. JS sends 1.0 for
// "decay" but C reads 0.0 from a NULL params and applies a different
// envelope). The discipline test in synth_param_schema_test.go locks the
// pairing.
func SynthParamSchemaIdentity() map[string]float64 {
	return map[string]float64{
		"pitch":       0,
		"decay":       1,
		"tone":        0,
		"drive":       0,
		"body":        0,
		"brightness":  0,
		"fundamental": 0, // 0 = "use the recipe's built-in default"
	}
}

// ── Modular voice param block ────────────────────────────────────────────────
//
// modularParamSchema is the SECOND canonical param block, for the unified
// modular synth voice (src/c/modular.h `modular_params`). It is independent of
// synthParamSchema so the legacy 7-field block — and every drum/FM golden —
// stays byte-identical. The order MUST mirror the C struct field declaration
// order because the WASM bridge fills heap[MODULAR_PARAM_INDEX[name]] and
// passes it as a const modular_params*.
//
// Adding/removing a modular_params field requires: edit src/c/modular.h, the
// ModularParams Go struct + recipeParamsToModular (modular_c.go), THIS slice,
// the identity map below, and re-run gen-synth-abi. The drift test in
// synth_param_schema_test.go locks the gen file.
// modularGenSlots and modularGenSlotFields define the Phase-1 gen bank:
// append-only by-field arrays (gen1_<f>..gen12_<f> contiguous per field) so
// later phases can append new per-slot fields without reordering existing
// flat indices. Identities are all no-ops: a preset that never touches the
// gen bank renders byte-identically to the pre-gen-bank engine.
const modularGenSlots = 12

// field name -> identity. Order matters (ABI layout); keep as a slice.
var modularGenSlotFields = []struct {
	Name     string
	Identity float64
}{
	{"source", 0}, {"wave", 0}, {"freq_mode", 0}, {"freq", 1}, {"gain", 1},
	{"env_fast_rate", 0}, {"env_tail_rate", 0}, {"env_fast_mix", 1}, {"env_tail_mix", 0},
	{"filt_type", 0}, {"filt_alpha", 0}, {"filt_freq", 1000}, {"filt_q", 0.707},
	{"phase_mode", 0}, {"phase", 0}, {"noise_offset", 0},
	// ── Phase-2 (bass family): analytic-voice (source==4) per-slot columns.
	// APPEND-ONLY after every existing slot field — identities are no-ops when
	// source != 4 so pre-Phase-2 presets stay byte-identical. ──
	{"pitch_env_amt", 0}, {"pitch_env_rate", 0}, {"harm_mix", 0},
	{"atk_amt", 0}, {"atk_rate", 0}, {"sat_k", 0}, {"out_scale", 1},
}

// modularGlobalsPhase2 are the Phase-2 (bass family) global params appended at
// the very END of the modular schema (after every gen-bank per-slot array).
// APPEND-ONLY. Identities are no-ops: voice_freq_hz=0 keeps the pow derivation;
// post_enabled=0 skips the shared POST stage; post_decay=1 is a decay no-op;
// the post_*_on gates default to 1 (matching post_config defaults) but only
// take effect when post_enabled>=0.5.
var modularGlobalsPhase2 = []struct {
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

// modularGenSlotFieldsPhase2KS are the Phase-2 Task-2 (Karplus-Strong, source==3)
// per-slot fields. They are appended at the VERY TAIL of the modular schema —
// AFTER the Phase-2 globals — because the KS slot voice was added after both the
// analytic source columns and the globals. Putting them here (rather than back
// in modularGenSlotFields, which precedes the globals) keeps every pre-existing
// flat index frozen: append-only ABI. Identities are 0 (read only when a slot's
// source is 3); the KS voice's kp_get(field, legacy_literal) supplies the exact
// double default (0.996 sustain / 0.35 pluck) at NaN.
var modularGenSlotFieldsPhase2KS = []struct {
	Name     string
	Identity float64
}{
	{"ks_sustain", 0}, {"ks_pluck", 0}, {"ks_blow", 0},
}

// modularGenSlotFieldsPhase3Kick are the Phase-3 (kick family) harmonic-bank
// kick voice (source==5) per-slot fields. APPEND-ONLY at the VERY tail of the
// modular schema — AFTER the Phase-2 KS columns — because the kick voice landed
// after every prior column (append-only ABI keeps every pre-existing flat index
// frozen). Identities are 0 (read only when a slot's source is 5): kick_variant
// is a discriminator the binding sets explicitly per recipe; the curated kick
// knobs (h2/h3/h4/env0/env1/pe_amt/pe_rate/click/noise) are NaN-driven at the
// binding so the C kp_get(field, legacy_literal) supplies the exact per-variant
// double default.
var modularGenSlotFieldsPhase3Kick = []struct {
	Name     string
	Identity float64
}{
	{"kick_variant", 0},
	{"kick_h2", 0}, {"kick_h3", 0}, {"kick_h4", 0},
	{"kick_env0", 0}, {"kick_env1", 0},
	{"kick_pe_amt", 0}, {"kick_pe_rate", 0},
	{"kick_click", 0}, {"kick_noise", 0},
}

// modularGlobalsPhase3 are the Phase-3 (kick post-order fix) global params
// appended at the VERY END of the modular schema — AFTER the Phase-3 kick
// per-slot columns — because post_order is the last field of modular_params (it
// landed after the kick voice columns). APPEND-ONLY keeps every prior flat index
// frozen. Identity 0 = the shared apply_post_params op order (a no-op selector);
// 1 selects the legacy base-kick order (drive before body).
var modularGlobalsPhase3 = []struct {
	Name     string
	Identity float64
}{
	{"post_order", 0},
}

// modularGenSlotFieldsPhase4Tom are the Phase-4 (tom family) 808-style tom voice
// (source==6) per-slot fields. APPEND-ONLY at the VERY tail of the modular schema
// — AFTER the Phase-3 globals (post_order) — because the tom voice landed after
// every prior column (append-only ABI keeps every pre-existing flat index
// frozen). Identities are 0 (read only when a slot's source is 6): tom_variant is
// a discriminator (0=tom 1=high 2=low) the binding sets explicitly per recipe;
// the curated tom knobs (sweep/ring/o1/o2/stick/room) are NaN-driven at the
// binding so the C kp_get(field, legacy_literal) supplies the exact per-variant
// double default.
var modularGenSlotFieldsPhase4Tom = []struct {
	Name     string
	Identity float64
}{
	{"tom_variant", 0},
	{"tom_sweep", 0}, {"tom_ring", 0},
	{"tom_o1", 0}, {"tom_o2", 0},
	{"tom_stick", 0}, {"tom_room", 0},
}

// modularGenSlotFieldsPhase5Snare are the Phase-5 (snare family) snare-ish voice
// (source==7, 3 variant branches) + clap voice (source==8) per-slot fields.
// APPEND-ONLY at the VERY tail of the modular schema — AFTER the Phase-4 tom
// columns — because the snare/clap voices landed after every prior column
// (append-only ABI keeps every pre-existing flat index frozen). Identities are 0
// (read only when a slot's source is 7 or 8): snare_variant is a discriminator
// (0=snare 1=rimshot 2=sidestick) the binding sets explicitly per recipe; the
// curated snare knobs (tone2/tune/tone_d/noise_d/tail_d/tone_m/noise_m/wire_m/
// attack) are NaN-driven at the binding so the C kp_get(field, legacy_literal)
// supplies the exact per-variant double default. snare_wave reuses gen_wave
// (NaN→0). The clap voice (source==8) reads tune/noise_d/tail_d/noise_m/attack
// from the SAME columns.
var modularGenSlotFieldsPhase5Snare = []struct {
	Name     string
	Identity float64
}{
	{"snare_variant", 0},
	{"snare_tone2", 0}, {"snare_tune", 0},
	{"snare_tone_d", 0}, {"snare_noise_d", 0}, {"snare_tail_d", 0},
	{"snare_tone_m", 0}, {"snare_noise_m", 0}, {"snare_wire_m", 0},
	{"snare_attack", 0},
}

// modularGenSlotFieldsPhase6Cymbal are the Phase-6 (cymbal family) metallic voice
// (source==9, 6 variant branches: 0=hihat 1=open-hihat 2=cowbell 3=shaker 4=ride
// 5=crash) per-slot fields. APPEND-ONLY at the VERY tail of the modular schema —
// AFTER the Phase-5 snare columns — because the cymbal voice landed after every
// prior column (append-only ABI keeps every pre-existing flat index frozen).
// Identities are 0 (read only when a slot's source is 9): cym_variant is a
// discriminator the binding sets explicitly per recipe; the curated cymbal knobs
// (tune/env_fast/env_tail/tone_m/noise_m/noise_d) are NaN-driven at the binding so
// the C kp_get(field, legacy_literal) supplies the exact per-variant double
// default. cym_wave reuses gen_wave (NaN→per-variant literal).
var modularGenSlotFieldsPhase6Cymbal = []struct {
	Name     string
	Identity float64
}{
	{"cym_variant", 0},
	{"cym_tune", 0}, {"cym_env_fast", 0}, {"cym_env_tail", 0},
	{"cym_tone_m", 0}, {"cym_noise_m", 0}, {"cym_noise_d", 0},
}

// modularGenSlotFieldsPhase7FM are the Phase-7 (FM family, the LAST legacy
// family) 4-operator FM preset voice (source==10, 5 variant branches: 0=bass
// 1=bell 2=lead 3=epiano 4=pluck) per-slot fields. APPEND-ONLY at the VERY tail
// of the modular schema — AFTER the Phase-6 cymbal columns — because the FM voice
// landed last (append-only ABI keeps every pre-existing flat index frozen).
// Identities are 0 (read only when a slot's source is 10): fm_variant is a
// discriminator the binding sets explicitly per recipe; the curated FM knobs
// (base/pe_amt/pe_decay/r1..r4/d1..d4/dec1..dec4) are NaN-driven at the binding so
// the C kp_get(field, legacy_literal) supplies the exact per-variant double
// default. fm_wave reuses gen_wave (NaN→0 sine).
var modularGenSlotFieldsPhase7FM = []struct {
	Name     string
	Identity float64
}{
	{"fm_variant", 0},
	{"fm_base", 0}, {"fm_pe_amt", 0}, {"fm_pe_decay", 0},
	{"fm_r1", 0}, {"fm_r2", 0}, {"fm_r3", 0}, {"fm_r4", 0},
	{"fm_d1", 0}, {"fm_d2", 0}, {"fm_d3", 0}, {"fm_d4", 0},
	{"fm_dec1", 0}, {"fm_dec2", 0}, {"fm_dec3", 0}, {"fm_dec4", 0},
}

// modularGlobalsPhase8 are the Phase-8C modulator stages (PITCH ENV / LFO /
// BURST — the spec-§1 stages closed by the gap audit) appended at the VERY END
// of the modular schema — AFTER the Phase-7 FM columns — so every prior flat
// index stays frozen (append-only ABI). All identities are 0: each *_enabled
// defaults DISABLED (the opposite polarity of the pre-existing stage toggles,
// because these stages are NEW — default-off is what keeps every pre-Phase-8C
// render byte-identical), and a disabled stage never reads its numeric knobs
// (exact bypass). The audible UI defaults (12 st sweep, 8 Hz / 0.5 wobble,
// clap-style bursts) live on the ParamDefs in modular_recipe.go, NOT here —
// schema identity is the engine-neutral fill for absent keys.
var modularGlobalsPhase8 = []struct {
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
	/* Phase-8E unison/ensemble. Identity: unison_voices=1 (single osc, no change). */
	{"unison_voices", 1}, {"unison_detune", 0}, {"unison_mix", 0.5},
	/* Phase-8F unison drift. Identity: both 0 = no drift, byte-identical. */
	{"unison_drift_rate", 0}, {"unison_drift_depth", 0},
	/* Phase-8G LFO onset delay. Identity: 0 = ramp factor 1.0 = byte-identical. */
	{"lfo_delay", 0},
	{"body_model", 0}, {"body_mix", 0}, {"bow_dynamics", 0},
}

// modularGenSlotFieldsPhase9KickExtra are the Phase-9 (kick family extras)
// per-slot fields — the source==5 kick voice's structural shaping constants
// (attack-boost amount, global-fade rate, saturation pre-gain) promoted to
// knobs so the configurable KICK stage can shape tail/punch/weight beyond the
// curated Phase-3 set. APPEND-ONLY at the VERY tail of the modular schema —
// AFTER the Phase-8C modulator globals — so every prior flat index stays frozen.
// Identities are 0 (read only when a slot's source is 5); the kick voice's
// kp_get(field, per-variant literal) supplies the exact legacy double default at
// NaN, so every existing kick variant stays byte-identical.
var modularGenSlotFieldsPhase9KickExtra = []struct {
	Name     string
	Identity float64
}{
	{"kick_attack", 0}, {"kick_fade", 0}, {"kick_sat", 0},
}

// modularGlobalsPhase10 is the Phase-10 KICK-stage enable: a single global
// gating the source==5 kick voice (the Synth-tab KICK stage's enable pill).
// APPEND-ONLY at the VERY tail of the modular schema — AFTER the Phase-9
// kick-extra columns. Identity 0 (off = the new-stage byte-identity polarity);
// every source==5 consumer (legacy kick binding/push + the dnb-kick seed) sets
// it to 1, and a render with no kick slot never reads it, so pre-Phase-10
// renders stay byte-identical.
var modularGlobalsPhase10 = []struct {
	Name     string
	Identity float64
}{
	{"kick_enabled", 0},
}

var modularParamSchema = func() []string {
	s := []string{
		"osc_type", "osc_detune", "osc_octave",
		"fm_algorithm",
		"fm_op1_ratio", "fm_op2_ratio", "fm_op3_ratio", "fm_op4_ratio",
		"fm_op1_depth", "fm_op2_depth", "fm_op3_depth", "fm_op4_depth",
		"fm_op1_level", "fm_op2_level", "fm_op3_level", "fm_op4_level",
		"amp_attack", "amp_decay", "amp_sustain", "amp_release", "amp_curve",
		"filter_type", "filter_cutoff", "filter_resonance",
		"drive", "pitch", "gain",
		// Per-stage bypass toggles (1=enabled) + the engine-internal noise seed.
		// APPENDED in C struct order — never reorder above this line.
		"osc_enabled", "fm_enabled", "env_enabled", "filter_enabled", "drive_enabled",
		"noise_seed",
		// ── Phase-1 gen bank (modular unification). Append-only from here. ──
		"noise_draws", "noise_prelude",
	}
	for _, f := range modularGenSlotFields {
		for k := 1; k <= modularGenSlots; k++ {
			s = append(s, fmt.Sprintf("gen%d_%s", k, f.Name))
		}
	}
	// Phase-2 globals append after EVERY gen-bank array (flat schema tail).
	for _, g := range modularGlobalsPhase2 {
		s = append(s, g.Name)
	}
	// Phase-2 Task-2 KS per-slot columns append at the VERY tail (after the
	// globals) — append-only, so the globals' flat indices stay frozen.
	for _, f := range modularGenSlotFieldsPhase2KS {
		for k := 1; k <= modularGenSlots; k++ {
			s = append(s, fmt.Sprintf("gen%d_%s", k, f.Name))
		}
	}
	// Phase-3 kick per-slot columns append after the KS columns (new tail).
	for _, f := range modularGenSlotFieldsPhase3Kick {
		for k := 1; k <= modularGenSlots; k++ {
			s = append(s, fmt.Sprintf("gen%d_%s", k, f.Name))
		}
	}
	// Phase-3 globals (post_order) append after the kick per-slot columns.
	for _, g := range modularGlobalsPhase3 {
		s = append(s, g.Name)
	}
	// Phase-4 tom per-slot columns append at the VERY END — after the Phase-3
	// globals — matching the C struct's last fields. Append-only.
	for _, f := range modularGenSlotFieldsPhase4Tom {
		for k := 1; k <= modularGenSlots; k++ {
			s = append(s, fmt.Sprintf("gen%d_%s", k, f.Name))
		}
	}
	// Phase-5 snare per-slot columns append at the VERY END (after the Phase-4
	// tom columns) — matching the C struct's last fields. Append-only.
	for _, f := range modularGenSlotFieldsPhase5Snare {
		for k := 1; k <= modularGenSlots; k++ {
			s = append(s, fmt.Sprintf("gen%d_%s", k, f.Name))
		}
	}
	// Phase-6 cymbal per-slot columns append at the VERY END (after the Phase-5
	// snare columns) — matching the C struct's last fields. Append-only.
	for _, f := range modularGenSlotFieldsPhase6Cymbal {
		for k := 1; k <= modularGenSlots; k++ {
			s = append(s, fmt.Sprintf("gen%d_%s", k, f.Name))
		}
	}
	// Phase-7 FM per-slot columns append at the VERY END (after the Phase-6
	// cymbal columns) — matching the C struct's last fields. Append-only.
	for _, f := range modularGenSlotFieldsPhase7FM {
		for k := 1; k <= modularGenSlots; k++ {
			s = append(s, fmt.Sprintf("gen%d_%s", k, f.Name))
		}
	}
	// Phase-8C modulator-stage globals (PITCH ENV / LFO / BURST) append at the
	// VERY END (after the Phase-7 FM columns) — matching the C struct's last
	// fields. Append-only.
	for _, g := range modularGlobalsPhase8 {
		s = append(s, g.Name)
	}
	// Phase-9 kick-extra per-slot columns append at the VERY END (after the
	// Phase-8C globals) — matching the C struct's last fields. Append-only.
	for _, f := range modularGenSlotFieldsPhase9KickExtra {
		for k := 1; k <= modularGenSlots; k++ {
			s = append(s, fmt.Sprintf("gen%d_%s", k, f.Name))
		}
	}
	// Phase-10 KICK-stage enable global appends at the VERY END (after the
	// Phase-9 kick-extra columns) — matching the C struct's last field.
	for _, g := range modularGlobalsPhase10 {
		s = append(s, g.Name)
	}
	return s
}()

// ModularParamSchema returns a fresh copy of the modular block's ordered names.
func ModularParamSchema() []string {
	out := make([]string, len(modularParamSchema))
	copy(out, modularParamSchema)
	return out
}

// ModularParamSchemaIndexMap returns paramName → struct index for the modular
// block (audio.js write-loop driver).
func ModularParamSchemaIndexMap() map[string]int {
	out := make(map[string]int, len(modularParamSchema))
	for i, name := range modularParamSchema {
		out[name] = i
	}
	return out
}

// ── FM family param block (DELETED) ──────────────────────────────────────────
//
// fmParamSchema / FMParamSchema* deleted: the FM family migrated to the modular
// engine (Phase-7, the LAST legacy family), so there is no separate fm_params ABI
// block. FM knobs travel in the wide modular_params block (gen_fm_* gen-slot
// columns, modularGenSlotFieldsPhase7FM) via the Go-side binding; the JS side
// fills the modular block (paramBlock:'modular'), not an fm family block. The
// shared fm_render core + fm_preset type remain in fmsynth.c/.h (used by both the
// modular osc_type==4 FM stage and the source==10 FM-family voice).

// ── Kick family param block (DELETED) ────────────────────────────────────────
//
// kickParamSchema / KickParamSchema* deleted: the kick family migrated to the
// modular engine (Phase-3), so there is no separate kick_params ABI block. Kick
// knobs travel in the wide modular_params block (gen_kick_* gen-slot columns,
// modularGenSlotFieldsPhase3Kick) via the Go-side binding; the JS side fills the
// modular block (paramBlock:'modular'), not a kick family block.

// ── Snare family param block (DELETED) ───────────────────────────────────────
//
// snareParamSchema / SnareParamSchema* deleted: the snare family migrated to the
// modular engine (Phase-5), so there is no separate snare_params ABI block. Snare
// knobs travel in the wide modular_params block (gen_snare_* gen-slot columns,
// modularGenSlotFieldsPhase5Snare) via the Go-side binding; the JS side fills the
// modular block (paramBlock:'modular'), not a snare family block.

// ── Cymbal family param block (DELETED) ──────────────────────────────────────
//
// cymbalParamSchema / CymbalParamSchema* deleted: the cymbal family migrated to
// the modular engine (Phase-6), so there is no separate cymbal_params ABI block.
// Cymbal knobs travel in the wide modular_params block (gen_cym_* gen-slot
// columns, modularGenSlotFieldsPhase6Cymbal) via the Go-side binding; the JS side
// fills the modular block (paramBlock:'modular'), not a cymbal family block.

// ── Bass family param block (DELETED) ────────────────────────────────────────
//
// bassParamSchema / BassParamSchema* deleted: the bass family migrated to the
// modular engine (Phase-2), so there is no separate bass_params ABI block. Bass
// knobs travel in the wide modular_params block via the Go-side binding; the JS
// side fills the modular block (paramBlock:'modular'), not a bass family block.

// ── Tom family param block (DELETED) ─────────────────────────────────────────
//
// tomParamSchema / TomParamSchema* deleted: the tom family migrated to the
// modular engine (Phase-4), so there is no separate tom_params ABI block. Tom
// knobs travel in the wide modular_params block (gen_tom_* gen-slot columns,
// modularGenSlotFieldsPhase4Tom) via the Go-side binding; the JS side fills the
// modular block (paramBlock:'modular'), not a tom family block.

// ModularParamSchemaIdentity returns the fill value for each modular param when
// the caller omits it. Unlike the post-process synth block (where identity is a
// no-op), the modular block's identities are the voice's BUILT-IN DEFAULTS —
// they define the sound. They MUST match the C NULL-defaults (mp_get fallbacks
// in modular.c) and the ModularSynthParamDefs defaults.
func ModularParamSchemaIdentity() map[string]float64 {
	out := map[string]float64{
		"osc_type": 0, "osc_detune": 0, "osc_octave": 0,
		"fm_algorithm": 0,
		"fm_op1_ratio": 1, "fm_op2_ratio": 1, "fm_op3_ratio": 1, "fm_op4_ratio": 1,
		"fm_op1_depth": 0, "fm_op2_depth": 0, "fm_op3_depth": 0, "fm_op4_depth": 0,
		"fm_op1_level": 1, "fm_op2_level": 0, "fm_op3_level": 0, "fm_op4_level": 0,
		"amp_attack": 0.005, "amp_decay": 0.3, "amp_sustain": 0.6, "amp_release": 0.2, "amp_curve": 1,
		"filter_type": 0, "filter_cutoff": 8000, "filter_resonance": 0.707,
		"drive": 0, "pitch": 0, "gain": 1,
		// Bypass toggles default to 1.0 (enabled) so an unedited modular voice
		// renders byte-identically to the pre-toggle engine (matches the
		// mp_get fallback of 1.0 in modular.c). noise_seed default 0.
		"osc_enabled": 1, "fm_enabled": 1, "env_enabled": 1, "filter_enabled": 1, "drive_enabled": 1,
		"noise_seed": 0,
	}
	// ── Phase-1 gen bank: every identity is a no-op so an untouched preset
	// renders byte-identically (see ModularSynthParamDefs + modular_stages.c).
	out["noise_draws"] = 1
	out["noise_prelude"] = 0
	for _, f := range modularGenSlotFields {
		for k := 1; k <= modularGenSlots; k++ {
			out[fmt.Sprintf("gen%d_%s", k, f.Name)] = f.Identity
		}
	}
	// Phase-2 globals: voice-freq override + shared POST stage (all no-ops).
	for _, g := range modularGlobalsPhase2 {
		out[g.Name] = g.Identity
	}
	// Phase-2 Task-2 KS per-slot columns (tail; identity 0, read only at source==3).
	for _, f := range modularGenSlotFieldsPhase2KS {
		for k := 1; k <= modularGenSlots; k++ {
			out[fmt.Sprintf("gen%d_%s", k, f.Name)] = f.Identity
		}
	}
	// Phase-3 kick per-slot columns (new tail; identity 0, read only at source==5).
	for _, f := range modularGenSlotFieldsPhase3Kick {
		for k := 1; k <= modularGenSlots; k++ {
			out[fmt.Sprintf("gen%d_%s", k, f.Name)] = f.Identity
		}
	}
	// Phase-3 globals (post_order): identity 0 selects the shared
	// apply_post_params op order (the no-op selector).
	for _, g := range modularGlobalsPhase3 {
		out[g.Name] = g.Identity
	}
	// Phase-4 tom per-slot columns (new tail; identity 0, read only at source==6).
	for _, f := range modularGenSlotFieldsPhase4Tom {
		for k := 1; k <= modularGenSlots; k++ {
			out[fmt.Sprintf("gen%d_%s", k, f.Name)] = f.Identity
		}
	}
	// Phase-5 snare per-slot columns (new tail; identity 0, read only at
	// source==7 (snare/rimshot/sidestick) or source==8 (clap)).
	for _, f := range modularGenSlotFieldsPhase5Snare {
		for k := 1; k <= modularGenSlots; k++ {
			out[fmt.Sprintf("gen%d_%s", k, f.Name)] = f.Identity
		}
	}
	// Phase-6 cymbal per-slot columns (new tail; identity 0, read only at
	// source==9 (hihat/open-hihat/cowbell/shaker/ride/crash)).
	for _, f := range modularGenSlotFieldsPhase6Cymbal {
		for k := 1; k <= modularGenSlots; k++ {
			out[fmt.Sprintf("gen%d_%s", k, f.Name)] = f.Identity
		}
	}
	// Phase-7 FM per-slot columns (new tail; identity 0, read only at source==10
	// (bass/bell/lead/epiano/pluck)).
	for _, f := range modularGenSlotFieldsPhase7FM {
		for k := 1; k <= modularGenSlots; k++ {
			out[fmt.Sprintf("gen%d_%s", k, f.Name)] = f.Identity
		}
	}
	// Phase-8C modulator-stage globals (new tail; identity 0 = stage DISABLED,
	// numerics unread). NOTE: unlike every pre-Phase-8C entry, the ParamDef
	// defaults for the modulator NUMERICS deliberately differ from these
	// identities (audible starting points, gated by the off-by-default enable) —
	// see modular_recipe.go and the carve-out in the defaults-match invariant.
	for _, g := range modularGlobalsPhase8 {
		out[g.Name] = g.Identity
	}
	// Phase-9 kick-extra per-slot columns (new tail; identity 0, read only at
	// source==5; the C kp_get supplies the per-variant literal at NaN).
	for _, f := range modularGenSlotFieldsPhase9KickExtra {
		for k := 1; k <= modularGenSlots; k++ {
			out[fmt.Sprintf("gen%d_%s", k, f.Name)] = f.Identity
		}
	}
	// Phase-10 KICK-stage enable (new tail; identity 0 = off).
	for _, g := range modularGlobalsPhase10 {
		out[g.Name] = g.Identity
	}
	return out
}

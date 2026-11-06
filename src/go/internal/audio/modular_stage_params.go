package audio

// modular_stage_params.go is the BUILD-TAG-NEUTRAL Phase-8A surface that makes
// every migrated recipe's modular pipeline stages USER-CONTROLLABLE. It owns:
//
//   - modularStageParamDefs: the user-facing stage ParamDefs appended to every
//     migrated recipe's schema (osc/env/filter + the post DRIVE/gain knobs +
//     the per-stage enable toggles), REUSED from ModularSynthParamDefs so bounds
//     / labels / groups / enums never drift,
//   - migratedStageDefaults: the off/identity stage defaults a migrated recipe
//     ships with, so the natural sound = pure family voice + POST, byte-identical
//     to before Phase-8A (osc/env/filter/drive OFF, every numeric at identity),
//   - the stage-name → modular-field mapping shared by the native binding
//     (applyUserStageParamsToModular, !test && !js) and the WASM push
//     (applyUserStageParamsToPush), so both render the SAME effective params,
//   - isModularStageParamName, the predicate the oracle sweep + the
//     every-wired-param-mutates effect test use to recognise a stage param.
//
// Why a curated subset of ModularSynthParamDefs (not the whole thing): the
// migrated recipes already carry the FM voice + POST as family/generic knobs.
// The modular FM stage osc (fm_op*_ratio etc.) COLLIDES by name with the FM
// recipes' family knobs. modularStageParamDefs therefore excludes any stage
// def whose name is already present on the recipe, AND drops a stage's enable
// toggle when its numeric knobs were all excluded (no silent no-op toggle).
// The fm_* group is excluded wholesale on the FM recipes (their voice IS FM).
// The modular DRIVE saturator stage (`drive`/`drive_enabled`) is simply NOT in
// modularStageGroups at all — deliberately unexposed in Phase 8A because its
// knob name is the same token as the generic POST `drive` every migrated
// recipe may wire; grit comes from the generic POST drive. (Deferred, not a
// collision-mechanism outcome.)

// modularStageGroup names one collapsible pipeline stage: the modular field
// names it owns (numeric knobs, NOT the toggle) and the toggle that gates it.
// Toggle "" means the group has no enable toggle (e.g. the standalone output
// gain).
type modularStageGroup struct {
	numeric []string
	toggle  string
}

// modularStageGroups is the ordered list of user-facing stages appended to a
// migrated recipe, in UI order (osc → fm → env → filter → drive/post). Names
// are looked up in ModularSynthParamDefs so the appended def reuses the exact
// bounds/labels/groups/enums; only the Default is overridden from stageDefaults.
//
// EXCLUDED vs ModularSynthParamDefs: `pitch` (collides with the legacy generic
// post `pitch` every drum recipe already carries; the modular `pitch` stays
// engine-internal for migrated recipes), `noise_seed` / hidden, and the gen_*
// bank (engine plumbing).
var modularStageGroups = []modularStageGroup{
	{numeric: []string{"osc_type", "osc_detune", "osc_octave"}, toggle: "osc_enabled"},
	{numeric: []string{
		"fm_algorithm",
		"fm_op1_ratio", "fm_op2_ratio", "fm_op3_ratio", "fm_op4_ratio",
		"fm_op1_depth", "fm_op2_depth", "fm_op3_depth", "fm_op4_depth",
		"fm_op1_level", "fm_op2_level", "fm_op3_level", "fm_op4_level",
	}, toggle: "fm_enabled"},
	{numeric: []string{"amp_attack", "amp_decay", "amp_sustain", "amp_release", "amp_curve"}, toggle: "env_enabled"},
	// ── Phase-8C modulator stages (PITCH ENV / LFO / BURST — the spec-§1 gap
	// closure). Unlike the pre-existing stages these default DISABLED on every
	// recipe (they are new DSP; off = byte-identity). Their numeric defaults are
	// the canonical audible ParamDef defaults, NOT schema identities — see
	// migratedStageDefaults's modulator carve-out. PITCH ENV is context-dependent
	// (needs the OSC stage on with a pitched osc type) exactly like fm_enabled;
	// LFO/BURST are post-mix and shape every voice incl. the family gen-slots. ──
	{numeric: []string{"pitchenv_amt", "pitchenv_decay"}, toggle: "pitchenv_enabled"},
	{numeric: []string{"lfo_rate", "lfo_depth", "lfo_target", "lfo_delay"}, toggle: "lfo_enabled"},
	{numeric: []string{
		"burst_sharp",
		"burst1_off", "burst1_amp", "burst2_off", "burst2_amp",
		"burst3_off", "burst3_amp", "burst4_off", "burst4_amp",
	}, toggle: "burst_enabled"},
	{numeric: []string{"filter_type", "filter_cutoff", "filter_resonance"}, toggle: "filter_enabled"},
	{numeric: []string{"filtenv_amt", "filtenv_decay", "filtenv_attack"}, toggle: "filtenv_enabled"},
	/* Phase-8E/F unison/ensemble + drift, PLUS the Phase-15 humanization knobs
	   (ens_scatter/vib_rate/vib_depth/humanize — same ENSEMBLE stage as the
	   unison stack, Task 8) AND the Phase-16 ens_jitter cycle-roughness knob.
	   No toggle — voices=1 is the identity off-state. */
	{numeric: []string{
		"unison_voices", "unison_detune", "unison_mix", "unison_drift_rate", "unison_drift_depth",
		"ens_scatter", "ens_vib_rate", "ens_vib_depth", "ens_humanize", "ens_jitter",
	}, toggle: ""},
	// Phase-15 FORMANT vowel-bank stage (Task 8), PLUS the Phase-16 formant_dry
	// dry-blend scaler. formant_enabled is the pill; the numerics are the
	// vowel/voice-type/mix/shape knobs.
	{numeric: []string{
		"formant_vowel", "formant_voice_type", "formant_mix", "formant_shift",
		"formant_breath", "formant_sing", "formant_morph_rate", "formant_morph_to",
		"formant_dry",
	}, toggle: "formant_enabled"},
	// Phase-15 RESONATOR body bank (Task 8). No toggle — body_model==0 ("Off")
	// is the identity off-state, mirroring the unison/ensemble stage.
	{numeric: []string{"body_model", "body_mix", "bow_dynamics"}, toggle: ""},
	// The modular DRIVE stage (`drive` + drive_enabled) is intentionally NOT
	// exposed on migrated recipes: its `drive` knob name is the SAME token the
	// generic post drive uses, and the two are different DSP nodes — surfacing
	// both would create a confusing duplicate-named knob (and break the
	// "drive == generic post drive" invariant the registry discipline tests
	// enforce). Migrated recipes keep grit via their generic `drive` (POST);
	// the extra modular saturator stays engine-internal for Phase 8A.
	//
	// WAV-clone stages: MODAL (the measured mode table's macros), STRIKE (the
	// mallet) and NOISE (the residual bands' level). Each has its pill; present
	// only on documents that declare the stage (FlatFieldDeclared).
	{numeric: []string{"modal_tune", "modal_stretch", "modal_decay_x", "modal_bright", "modal_keep",
		"modal_law_a", "modal_law_p", "modal_law_b"}, toggle: "modal_enabled"},
	{numeric: []string{"strike_tc", "strike_hardness", "strike_click"}, toggle: "strike_enabled"},
	{numeric: []string{"noiseband_level"}, toggle: "noiseband_enabled"},
	// Standalone output trim. No enable toggle of its own.
	{numeric: []string{"gain"}, toggle: ""},
	// FINISH: the post ops + output limit. Always on (each op is a no-op at its
	// identity), so no toggle.
	{numeric: []string{"post_pitch", "post_decay", "post_decay_rate", "post_body",
		"post_brightness", "post_darken", "post_drive", "out_clamp"}, toggle: ""},
}

// modularStageParamNames is every name modularStageGroups can emit (numerics +
// toggles), used by isModularStageParamName. Built once at init.
var modularStageParamNames = func() map[string]bool {
	m := map[string]bool{}
	for _, g := range modularStageGroups {
		for _, n := range g.numeric {
			m[n] = true
		}
		if g.toggle != "" {
			m[g.toggle] = true
		}
	}
	return m
}()

// isModularStageParamName reports whether name is one of the Phase-8A stage
// params appended to migrated recipes. The legacy-oracle sweep skips these (the
// legacy renderer had no such capability, so there is no oracle to pin them
// against), and the every-wired-param-mutates effect test skips them too (their
// effect is context-dependent — osc_type does nothing until osc_enabled=1 —
// covered instead by the dedicated context-aware stage_params_test.go).
func isModularStageParamName(name string) bool {
	return modularStageParamNames[name]
}

// IsStageParamName reports whether name is a standardized stage param (so the
// Synth tab routes it by stage group and grows the stage's enable pill) rather
// than a family/generic knob.
func IsStageParamName(name string) bool { return isModularStageParamName(name) }

// modularStageDefByName returns the user-facing ParamDef for a stage param name,
// reusing the matching ModularSynthParamDefs entry verbatim (bounds / label /
// group / enum). Returns ok=false for an unknown name. Built from a cached index so the schema
// is walked once.
func modularStageDefByName(name string) (ParamDef, bool) {
	d, ok := modularSchemaDefIndex[name]
	return d, ok
}

// modularSchemaDefIndex indexes ModularSynthParamDefs by name (the canonical
// bounds/labels/groups/enums source). Built once at init.
var modularSchemaDefIndex = func() map[string]ParamDef {
	m := map[string]ParamDef{}
	for _, d := range ModularSynthParamDefs() {
		m[d.Name] = d
	}
	return m
}()

// StageParamDefByName is the exported form of modularStageDefByName for the UI:
// returns the canonical user-facing ParamDef (bounds / label / group / enum)
// for a standardized stage param name, so the section router can read its Group
// without a registry lookup. ok=false for a non-stage name.
func StageParamDefByName(name string) (ParamDef, bool) {
	return modularStageDefByName(name)
}

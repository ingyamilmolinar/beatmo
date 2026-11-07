package audio

// The user-facing pipeline stages: ParamDefs REUSED from ModularSynthParamDefs so bounds, labels and enums never drift.
// A curated subset: the modular FM stage collides by name with the FM family's knobs, and the modular `drive`
// saturator shares its name with the POST drive, so it stays unexposed.

// toggle "" means the stage has no enable toggle.
type modularStageGroup struct {
	numeric []string
	toggle  string
}

// In UI order; only the Default is overridden. Excludes `pitch` (collides with the generic post pitch), noise_seed
// and the gen_* bank.
var modularStageGroups = []modularStageGroup{
	{numeric: []string{"osc_type", "osc_detune", "osc_octave"}, toggle: "osc_enabled"},
	{numeric: []string{
		"fm_algorithm",
		"fm_op1_ratio", "fm_op2_ratio", "fm_op3_ratio", "fm_op4_ratio",
		"fm_op1_depth", "fm_op2_depth", "fm_op3_depth", "fm_op4_depth",
		"fm_op1_level", "fm_op2_level", "fm_op3_level", "fm_op4_level",
	}, toggle: "fm_enabled"},
	{numeric: []string{"amp_attack", "amp_decay", "amp_sustain", "amp_release", "amp_curve"}, toggle: "env_enabled"},
	// Off by default on every instrument; their numeric defaults are audible starting points, not identities. PITCH ENV
	// needs a pitched OSC stage, like fm_enabled; LFO and BURST are post-mix.
	{numeric: []string{"pitchenv_amt", "pitchenv_decay"}, toggle: "pitchenv_enabled"},
	{numeric: []string{"lfo_rate", "lfo_depth", "lfo_target", "lfo_delay"}, toggle: "lfo_enabled"},
	{numeric: []string{
		"burst_sharp",
		"burst1_off", "burst1_amp", "burst2_off", "burst2_amp",
		"burst3_off", "burst3_amp", "burst4_off", "burst4_amp",
	}, toggle: "burst_enabled"},
	{numeric: []string{"filter_type", "filter_cutoff", "filter_resonance"}, toggle: "filter_enabled"},
	{numeric: []string{"filtenv_amt", "filtenv_decay", "filtenv_attack"}, toggle: "filtenv_enabled"},
	// No toggle: voices = 1 is the off state.
	{numeric: []string{
		"unison_voices", "unison_detune", "unison_mix", "unison_drift_rate", "unison_drift_depth",
		"ens_scatter", "ens_vib_rate", "ens_vib_depth", "ens_humanize", "ens_jitter",
	}, toggle: ""},
	// formant_enabled is the pill.
	{numeric: []string{
		"formant_vowel", "formant_voice_type", "formant_mix", "formant_shift",
		"formant_breath", "formant_sing", "formant_morph_rate", "formant_morph_to",
		"formant_dry",
	}, toggle: "formant_enabled"},
	// No toggle: body_model 0 ("Off") is the off state.
	{numeric: []string{"body_model", "body_mix", "bow_dynamics"}, toggle: ""},
	// The modular `drive` is not exposed: it shares the POST drive's name but is a different DSP node. The WAV-clone
	// stages appear only on documents that declare them.
	{numeric: []string{"modal_tune", "modal_stretch", "modal_decay_x", "modal_bright", "modal_keep",
		"modal_law_a", "modal_law_p", "modal_law_b"}, toggle: "modal_enabled"},
	{numeric: []string{"strike_tc", "strike_hardness", "strike_click"}, toggle: "strike_enabled"},
	{numeric: []string{"noiseband_level"}, toggle: "noiseband_enabled"},
	// Standalone output trim. No enable toggle of its own.
	{numeric: []string{"gain"}, toggle: ""},
	// Always on (each op is a no-op at its identity), so no toggle.
	{numeric: []string{"post_pitch", "post_decay", "post_decay_rate", "post_body",
		"post_brightness", "post_darken", "post_drive", "out_clamp"}, toggle: ""},
}

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

// Their effect is context-dependent (osc_type does nothing until osc_enabled = 1), so the generic every-param-mutates
// sweep skips them; context-aware tests cover them.
func isModularStageParamName(name string) bool {
	return modularStageParamNames[name]
}

// Routed by stage group (with the stage's enable pill) rather than as a family knob.
func IsStageParamName(name string) bool { return isModularStageParamName(name) }

func modularStageDefByName(name string) (ParamDef, bool) {
	d, ok := modularSchemaDefIndex[name]
	return d, ok
}

var modularSchemaDefIndex = func() map[string]ParamDef {
	m := map[string]ParamDef{}
	for _, d := range ModularSynthParamDefs() {
		m[d.Name] = d
	}
	return m
}()

// Lets the section router read the Group without a registry lookup.
func StageParamDefByName(name string) (ParamDef, bool) {
	return modularStageDefByName(name)
}

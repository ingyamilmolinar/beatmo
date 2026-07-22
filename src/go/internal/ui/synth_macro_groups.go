package ui

// synth_macro_groups.go is the UI-ONLY "macro layer" over the modular synth's
// raw params: a small set of plain-English "concept" buckets, each owning an
// ordered list of existing ParamDef names. It adds NO persisted state and NO
// audio-path change — every leaf still writes through audio.SetInstrumentParam.
// It is the data model behind the beginner-first drill-down UI (root shows the
// concept cards; drilling into one shows its params as the same cards).
//
// Design rules (from docs/superpowers/specs/2026-07-21-synth-macro-controls-proposal.md):
//   - MORE clean buckets beat fewer overloaded ones — each concept is ONE idea
//     (Attack ≠ Length, Width ≠ Human Feel, Movement ≠ Human Feel).
//   - Discrete character pickers (osc_type, filter_type, body_model, lfo_target,
//     formant_vowel/voice_type, fm_algorithm) live inside a concept but are never
//     fused into a continuous macro — they render as their existing enum picker.
//   - HIDE-UNUSED: every modular recipe exposes the SAME full schema, so a concept
//     is shown only when the instrument actually uses it (via enable flags / key
//     values), not merely because its params exist in the schema.
//
// v1 covers MELODIC recipes. Percussion/kick recipes (schemaUsesKickStage) return
// nil here and keep the existing stage UI; a kick concept set is a fast-follow.

import "github.com/ingyamilmolinar/beatmo/internal/audio"

// macroGraphKind selects which existing concept-viz graph a macro card draws
// (wired in a later task; the model carries it so the card can pick a visual).
type macroGraphKind int

const (
	macroGraphOsc macroGraphKind = iota
	macroGraphFilter
	macroGraphAttack
	macroGraphDecay
	macroGraphSpread
	macroGraphWavy
	macroGraphBumps
	macroGraphSoftclip
	macroGraphPitch
	macroGraphLevel
)

// macroGroup is a concrete, in-use concept bucket for one instrument: a label,
// a one-liner, a graph kind, and the ordered subset of its candidate params that
// are actually visible on this instrument.
type macroGroup struct {
	ID     string
	Label  string
	Gloss  string
	Graph  macroGraphKind
	Params []string // child leaf param names, in display order
}

// macroGroupDef is a table entry. Params lists every param the concept CAN own
// (across all instruments); the builder keeps only those present + visible on a
// given instrument. active decides HIDE-UNUSED (nil = always shown).
type macroGroupDef struct {
	ID     string
	Label  string
	Gloss  string
	Graph  macroGraphKind
	Params []string
	active func(m audio.RecipeParams) bool
}

// enableOn returns an active-predicate that is true when a stage's *_enabled flag
// (or any 0/1 gate param) is on in the merged params.
func enableOn(name string) func(audio.RecipeParams) bool {
	return func(m audio.RecipeParams) bool { return m[name] >= 0.5 }
}

// voicesGT1 gates the ensemble concepts (Width / Human Feel): only meaningful
// once the instrument stacks more than one voice.
func voicesGT1(m audio.RecipeParams) bool { return m["unison_voices"] >= 1.5 }

// bodyModelOn gates the Body concept: body_model 0 == "Off".
func bodyModelOn(m audio.RecipeParams) bool { return m["body_model"] >= 0.5 }

// macroGroupDefsMelodic is the ordered concept table for melodic instruments.
// Ordering is musical priority (what a beginner reaches for first). Every visible
// melodic param except the output `gain` (that is the row's volume, not a synth
// timbre control) is claimed by exactly one concept.
var macroGroupDefsMelodic = []macroGroupDef{
	{ID: "sound", Label: "Sound", Gloss: "The basic character of the sound.", Graph: macroGraphOsc, Params: []string{
		"osc_type", "osc_octave", "osc_detune", "pitch",
		"fm_algorithm", "fm_op1_ratio", "fm_op2_ratio", "fm_op3_ratio", "fm_op4_ratio",
		"fm_op1_depth", "fm_op2_depth", "fm_op3_depth", "fm_op4_depth",
		"fm_op1_level", "fm_op2_level", "fm_op3_level", "fm_op4_level",
		"osc_sax_blow", "osc_sax_reed_off", "osc_sax_reed_slope", "osc_sax_reflect", "osc_sax_breath", "osc_sax_loss",
		"osc_bow_pos", "osc_bow_slope", "osc_bow_vel", "osc_bow_loss",
	}},
	{ID: "brightness", Label: "Brightness", Gloss: "Dull and dark, or bright and sharp?", Graph: macroGraphFilter, Params: []string{
		"filter_cutoff", "filter_resonance", "filter_type", "filtenv_amt", "filtenv_decay", "filtenv_attack",
	}},
	{ID: "attack", Label: "Attack", Gloss: "Does it start instantly, or fade in?", Graph: macroGraphAttack, Params: []string{"amp_attack"}},
	{ID: "length", Label: "Length", Gloss: "A short pluck, or a long held note?", Graph: macroGraphDecay, Params: []string{
		"amp_decay", "amp_sustain", "amp_release", "amp_curve",
	}},
	{ID: "width", Label: "Width", Gloss: "One thin voice, or a big wide group?", Graph: macroGraphSpread, Params: []string{
		"unison_voices", "unison_detune", "unison_mix",
	}, active: voicesGT1},
	{ID: "humanfeel", Label: "Human Feel", Gloss: "Alive and human, or perfectly still?", Graph: macroGraphWavy, Params: []string{
		"unison_drift_rate", "unison_drift_depth", "ens_scatter", "ens_humanize", "ens_jitter", "ens_vib_rate", "ens_vib_depth",
	}, active: voicesGT1},
	{ID: "movement", Label: "Movement", Gloss: "A steady note, or a moving one?", Graph: macroGraphWavy, Params: []string{
		"lfo_rate", "lfo_depth", "lfo_target", "lfo_delay",
	}, active: enableOn("lfo_enabled")},
	{ID: "body", Label: "Body", Gloss: "How real and acoustic it sounds.", Graph: macroGraphBumps, Params: []string{
		"body_model", "body_mix", "bow_dynamics",
	}, active: bodyModelOn},
	{ID: "warmth", Label: "Warmth", Gloss: "Clean, or warm and gritty?", Graph: macroGraphSoftclip, Params: []string{"drive"}},
	{ID: "pitchdrop", Label: "Pitch Drop", Gloss: "A downward pitch punch, like an 808.", Graph: macroGraphPitch, Params: []string{
		"pitchenv_amt", "pitchenv_decay",
	}, active: enableOn("pitchenv_enabled")},
	{ID: "voice", Label: "Voice", Gloss: "The vowel and voice character.", Graph: macroGraphOsc, Params: []string{
		"formant_vowel", "formant_voice_type", "formant_mix", "formant_breath", "formant_sing",
		"formant_shift", "formant_morph_rate", "formant_morph_to", "formant_dry",
	}, active: enableOn("formant_enabled")},
	{ID: "flam", Label: "Flam", Gloss: "One hit, or a quick roll?", Graph: macroGraphWavy, Params: []string{
		"burst_sharp", "burst1_off", "burst1_amp", "burst2_off", "burst2_amp", "burst3_off", "burst3_amp", "burst4_off", "burst4_amp",
	}, active: enableOn("burst_enabled")},
}

// buildMacroGroups computes the in-use concept buckets for one instrument from
// its recipe schema + current (merged) param values. Returns nil for percussion
// recipes (v1 keeps their existing stage UI). Pure — no DrumView state — so it is
// unit-testable against RecipeDefaultParams.
func buildMacroGroups(schema []audio.ParamDef, merged audio.RecipeParams) []macroGroup {
	if schemaUsesKickStage(schema) {
		return nil // percussion/kick: fall back to the existing stage UI in v1
	}
	present := make(map[string]audio.ParamDef, len(schema))
	for _, d := range schema {
		present[d.Name] = d
	}
	const kickActive = false // melodic recipes only reach here
	oscType := merged["osc_type"]

	var out []macroGroup
	for _, def := range macroGroupDefsMelodic {
		var ps []string
		for _, name := range def.Params {
			d, ok := present[name]
			if !ok {
				continue
			}
			if isSynthEnableParam(name) || synthParamHiddenInContext(d, kickActive, oscType) {
				continue
			}
			ps = append(ps, name)
		}
		if len(ps) == 0 {
			continue
		}
		if def.active != nil && !def.active(merged) {
			continue // HIDE-UNUSED: the instrument doesn't use this concept
		}
		out = append(out, macroGroup{ID: def.ID, Label: def.Label, Gloss: def.Gloss, Graph: def.Graph, Params: ps})
	}
	return out
}

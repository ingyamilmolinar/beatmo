package ui

// synth_macro_groups.go is the UI-ONLY "category layer" over the synth's raw
// params: a small set of plain-English concept buckets, each owning an ordered
// list of existing ParamDef names. It adds NO persisted state and NO audio-path
// change — every leaf still writes through audio.SetInstrumentParam. It is the
// data model behind the Synth tab's overview → drill UI (root shows the category
// cards; drilling into one shows its params as ParamCards).
//
// Design rules (docs/superpowers/specs/2026-09-05-synth-tab-overview-and-signal-graph-design.md):
//   - EVERY recipe with a schema gets categories — no instrument has a
//     different flow. WAV/sample instruments (no recipe) never reach here.
//   - MORE clean buckets beat fewer overloaded ones — each concept is ONE idea
//     (Attack ≠ Length, Width ≠ Human Feel, Movement ≠ Human Feel).
//   - Discrete character pickers (osc_type, filter_type, body_model, lfo_target,
//     formant_vowel/voice_type, fm_algorithm, <family>_wave, kick variant) live
//     inside a concept but are never fused into a continuous macro — they
//     render as their existing enum picker.
//   - HIDE-UNUSED: every modular recipe exposes the SAME full schema, so a concept
//     is shown only when the instrument actually uses it (via enable flags / key
//     values), not merely because its params exist in the schema.
//   - Essentials: a category's params whose ParamDef.Tier is not Advanced, in
//     table order. The drilled view shows them first; the rest sit behind "More".
//
// Two concept tables exist — macroGroupDefsMelodic and macroGroupDefsPercussion —
// selected per instrument by synthCategoryTableFor(schema): kick-stage recipes
// and the bespoke drum families take the percussion table, everything else the
// melodic one.

import (
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
	"github.com/ingyamilmolinar/beatmo/internal/synthdial"
)

// macroGroup is a concrete, in-use concept bucket for one instrument: a label,
// a one-liner, the stage(s) it maps to, and the ordered subset of
// its candidate params that are actually visible on this instrument.
type macroGroup struct {
	ID     string
	Label  string
	Gloss  string
	Stages []synthSectionID
	Params []string // child param names, in display order
	// Essentials is the Params subset whose Tier is not Advanced (table order).
	// The drilled view shows these first; Params minus Essentials sit under More.
	Essentials []string
	// spriteID keys the root card's render cache ("cat:" + ID), built once.
	spriteID string
	// Dials are the card's macro dials live on this instrument (≤3, file
	// order; synth_dials.go). Each term is a param this card claims.
	Dials []synthdial.Resolved
}

// macroGroupDef is a table entry. Params lists every param the concept CAN own
// (across all instruments); the builder keeps only those present + visible on a
// given instrument. active decides HIDE-UNUSED (nil = always shown). Stages
// names the pipeline stage(s) the concept's params live in — a concept with
// exactly one stage shows that stage's enable pill when drilled.
type macroGroupDef struct {
	ID       string
	LabelKey i18n.Key
	GlossKey i18n.Key
	Stages   []synthSectionID
	Params   []string
	active   func(m audio.RecipeParams) bool
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

// nonZero reports whether any of the named params is non-zero in the merged map.
func nonZero(names ...string) func(audio.RecipeParams) bool {
	return func(m audio.RecipeParams) bool {
		for _, n := range names {
			if v := m[n]; v > 0.001 || v < -0.001 {
				return true
			}
		}
		return false
	}
}

// anyOf ORs active predicates.
func anyOf(fns ...func(audio.RecipeParams) bool) func(audio.RecipeParams) bool {
	return func(m audio.RecipeParams) bool {
		for _, f := range fns {
			if f(m) {
				return true
			}
		}
		return false
	}
}

// macroGroupDefsMelodic is the ordered concept table for melodic instruments
// (modular voices, the fm-* family, the base modular, pad, whisper). Ordering is
// musical priority (what a beginner reaches for first). Every visible param
// except the output `gain` (that is the row's volume, not a synth timbre
// control) is claimed by exactly one concept — guarded by
// TestMacroGroups_EveryInstrumentHasCategories.
var macroGroupDefsMelodic = []macroGroupDef{
	{ID: "sound", LabelKey: i18n.KeySynthCatSound, GlossKey: i18n.KeySynthCatSoundGloss,
		Stages: []synthSectionID{synthSectionOsc, synthSectionFM, synthSectionVoice, synthSectionFinish},
		Params: []string{"osc_type", "osc_octave", "osc_detune", "pitch", "post_pitch",
			"fm_algorithm", "fm_op1_ratio", "fm_op2_ratio", "fm_op3_ratio", "fm_op4_ratio",
			"fm_op1_depth", "fm_op2_depth", "fm_op3_depth", "fm_op4_depth",
			"fm_op1_level", "fm_op2_level", "fm_op3_level", "fm_op4_level",
			"osc_sax_blow", "osc_sax_reed_off", "osc_sax_reed_slope", "osc_sax_reflect", "osc_sax_breath", "osc_sax_loss",
			"osc_bow_pos", "osc_bow_slope", "osc_bow_vel", "osc_bow_loss"}},
	// The WAV-clone stages: a measured instrument's ringing modes (+ residual
	// noise) and the mallet that excites them.
	{ID: "ring", LabelKey: i18n.KeySynthCatModes, GlossKey: i18n.KeySynthCatModesGloss,
		Stages: []synthSectionID{synthSectionModal, synthSectionNoise},
		Params: []string{"modal_tune", "modal_decay_x", "modal_bright", "modal_keep", "noiseband_level",
			"modal_stretch", "modal_law_a", "modal_law_p", "modal_law_b"}, active: enableOn("modal_enabled")},
	{ID: "mallet", LabelKey: i18n.KeySynthCatMallet, GlossKey: i18n.KeySynthCatMalletGloss,
		Stages: []synthSectionID{synthSectionStrike},
		Params: []string{"strike_tc", "strike_hardness", "strike_click"}, active: enableOn("strike_enabled")},
	{ID: "brightness", LabelKey: i18n.KeySynthCatBrightness, GlossKey: i18n.KeySynthCatBrightnessGloss,
		Stages: []synthSectionID{synthSectionFilter, synthSectionFilterEnv, synthSectionFinish},
		Params: []string{"filter_cutoff", "filter_resonance", "filter_type",
			"filtenv_amt", "filtenv_decay", "filtenv_attack", "post_brightness", "post_darken"}},
	{ID: "attack", LabelKey: i18n.KeySynthCatAttack, GlossKey: i18n.KeySynthCatAttackGloss,
		Stages: []synthSectionID{synthSectionEnvelope},
		Params: []string{"amp_attack"}},
	{ID: "length", LabelKey: i18n.KeySynthCatLength, GlossKey: i18n.KeySynthCatLengthGloss,
		Stages: []synthSectionID{synthSectionEnvelope, synthSectionFinish},
		Params: []string{"amp_decay", "amp_sustain", "amp_release", "amp_curve", "post_decay", "post_decay_rate"}},
	{ID: "width", LabelKey: i18n.KeySynthCatWidth, GlossKey: i18n.KeySynthCatWidthGloss,
		Stages: []synthSectionID{synthSectionEnsemble},
		Params: []string{"unison_voices", "unison_detune", "unison_mix"}, active: voicesGT1},
	{ID: "humanfeel", LabelKey: i18n.KeySynthCatHumanFeel, GlossKey: i18n.KeySynthCatHumanFeelGloss,
		Stages: []synthSectionID{synthSectionEnsemble},
		Params: []string{"unison_drift_rate", "unison_drift_depth", "ens_scatter", "ens_humanize", "ens_jitter", "ens_vib_rate", "ens_vib_depth"}, active: voicesGT1},
	{ID: "movement", LabelKey: i18n.KeySynthCatMovement, GlossKey: i18n.KeySynthCatMovementGloss,
		Stages: []synthSectionID{synthSectionLFO},
		Params: []string{"lfo_rate", "lfo_depth", "lfo_target", "lfo_delay"}, active: enableOn("lfo_enabled")},
	{ID: "body", LabelKey: i18n.KeySynthCatBody, GlossKey: i18n.KeySynthCatBodyGloss,
		Stages: []synthSectionID{synthSectionResonator},
		Params: []string{"body_model", "body_mix", "bow_dynamics"}, active: bodyModelOn},
	{ID: "warmth", LabelKey: i18n.KeySynthCatWarmth, GlossKey: i18n.KeySynthCatWarmthGloss,
		Stages: []synthSectionID{synthSectionPost, synthSectionFinish},
		Params: []string{"drive", "post_drive", "post_body", "out_clamp"}},
	{ID: "pitchdrop", LabelKey: i18n.KeySynthCatPitchDrop, GlossKey: i18n.KeySynthCatPitchDropGloss,
		Stages: []synthSectionID{synthSectionPitch},
		Params: []string{"pitchenv_amt", "pitchenv_decay"},
		// nonZeroMacro covers the gen-bank spellings, so a family that becomes a
		// config seed keeps lighting this card without being named here.
		active: anyOf(enableOn("pitchenv_enabled"), nonZeroMacro("pitchdrop"))},
	{ID: "voice", LabelKey: i18n.KeySynthCatVoice, GlossKey: i18n.KeySynthCatVoiceGloss,
		Stages: []synthSectionID{synthSectionFormant},
		Params: []string{"formant_vowel", "formant_voice_type", "formant_mix", "formant_breath", "formant_sing",
			"formant_shift", "formant_morph_rate", "formant_morph_to", "formant_dry"}, active: enableOn("formant_enabled")},
	{ID: "flam", LabelKey: i18n.KeySynthCatFlam, GlossKey: i18n.KeySynthCatFlamGloss,
		Stages: []synthSectionID{synthSectionBurst},
		Params: []string{"burst_sharp", "burst1_off", "burst1_amp", "burst2_off", "burst2_amp", "burst3_off", "burst3_amp", "burst4_off", "burst4_amp"}, active: enableOn("burst_enabled")},
}

// macroGroupDefsPercussion is the ordered concept table for kick/percussion
// recipes: the modular kick-stage family (schemaUsesKickStage) AND the bespoke
// drum families (snare/clap/hihat/cymbal/tom/kick/sub-bass/conga). Modular kicks
// share the full modular schema PLUS the kick `gen1_kick_*` shaping stage; the
// bespoke drums carry their own family knobs (snare_*/cym_*/tom_*/bass_*/kick_*)
// plus the generic pitch/decay/tone/drive/body/brightness knobs. Ordering =
// what a beatmaker reaches for first: transient → body → pitch → tone → tail →
// engine → grit.
var macroGroupDefsPercussion = []macroGroupDef{
	{ID: "ring", LabelKey: i18n.KeySynthCatModes, GlossKey: i18n.KeySynthCatModesGloss,
		Stages: []synthSectionID{synthSectionModal, synthSectionNoise},
		Params: []string{"modal_tune", "modal_decay_x", "modal_bright", "modal_keep", "noiseband_level",
			"modal_stretch", "modal_law_a", "modal_law_p", "modal_law_b"}, active: enableOn("modal_enabled")},
	{ID: "mallet", LabelKey: i18n.KeySynthCatMallet, GlossKey: i18n.KeySynthCatMalletGloss,
		Stages: []synthSectionID{synthSectionStrike},
		Params: []string{"strike_tc", "strike_hardness", "strike_click"}, active: enableOn("strike_enabled")},
	{ID: "punch", LabelKey: i18n.KeySynthCatPunch, GlossKey: i18n.KeySynthCatPunchGloss,
		Stages: []synthSectionID{synthSectionVoice, synthSectionBurst, synthSectionEnvelope},
		Params: []string{"amp_attack", "burst_sharp", "burst1_off", "burst1_amp", "burst2_off", "burst2_amp", "burst3_off", "burst3_amp", "burst4_off", "burst4_amp"}},
	{ID: "boom", LabelKey: i18n.KeySynthCatBoom, GlossKey: i18n.KeySynthCatBoomGloss,
		Stages: []synthSectionID{synthSectionVoice, synthSectionResonator, synthSectionFinish},
		Params: []string{"pitch", "voice_freq_hz", "post_body",
			"body_model", "body_mix", "bow_dynamics"}},
	{ID: "pitchdrop", LabelKey: i18n.KeySynthCatPitchDrop, GlossKey: i18n.KeySynthCatPitchDropGloss,
		Stages: []synthSectionID{synthSectionPitch, synthSectionVoice, synthSectionFinish},
		Params: []string{"pitchenv_amt", "pitchenv_decay", "post_pitch"}, active: anyOf(enableOn("pitchenv_enabled"),
			nonZero("post_pitch"),
			nonZeroMacro("pitchdrop"))},
	{ID: "tone", LabelKey: i18n.KeySynthCatTone, GlossKey: i18n.KeySynthCatToneGloss,
		Stages: []synthSectionID{synthSectionFilter, synthSectionFilterEnv, synthSectionVoice, synthSectionFinish},
		Params: []string{"filter_cutoff", "filter_resonance", "filter_type", "post_brightness", "post_darken",
			"filtenv_amt", "filtenv_decay", "filtenv_attack"}},
	{ID: "length", LabelKey: i18n.KeySynthCatLength, GlossKey: i18n.KeySynthCatLengthPercGloss,
		Stages: []synthSectionID{synthSectionEnvelope, synthSectionVoice, synthSectionFinish},
		Params: []string{"amp_decay", "amp_sustain", "amp_release", "amp_curve", "post_decay", "post_decay_rate"}},
	{ID: "source", LabelKey: i18n.KeySynthCatSource, GlossKey: i18n.KeySynthCatSourceGloss,
		Stages: []synthSectionID{synthSectionOsc, synthSectionFM, synthSectionVoice},
		Params: []string{"osc_type", "osc_octave", "osc_detune",
			"fm_algorithm", "fm_op1_ratio", "fm_op2_ratio", "fm_op3_ratio", "fm_op4_ratio",
			"fm_op1_depth", "fm_op2_depth", "fm_op3_depth", "fm_op4_depth",
			"fm_op1_level", "fm_op2_level", "fm_op3_level", "fm_op4_level",
			"osc_sax_blow", "osc_sax_reed_off", "osc_sax_reed_slope", "osc_sax_reflect", "osc_sax_breath", "osc_sax_loss",
			"osc_bow_pos", "osc_bow_slope", "osc_bow_vel", "osc_bow_loss",
			"unison_voices", "unison_detune", "unison_mix", "unison_drift_rate", "unison_drift_depth",
			"ens_scatter", "ens_humanize", "ens_jitter", "ens_vib_rate", "ens_vib_depth",
			"lfo_rate", "lfo_depth", "lfo_target", "lfo_delay",
			"formant_vowel", "formant_voice_type", "formant_mix", "formant_breath", "formant_sing",
			"formant_shift", "formant_morph_rate", "formant_morph_to", "formant_dry"}},
	{ID: "grit", LabelKey: i18n.KeySynthCatGrit, GlossKey: i18n.KeySynthCatGritGloss,
		Stages: []synthSectionID{synthSectionPost, synthSectionFinish},
		Params: []string{"drive", "post_drive", "out_clamp"}},
}

// synthCategoryTableFor picks the concept table for a schema: percussion for
// the percussive gen-slot families, melodic otherwise.
func synthCategoryTableFor(schema []audio.ParamDef) []macroGroupDef {
	// A drum is identified by its gen-slot SOURCE (a Percussive family stage):
	// its knobs are gen1_<family>_*, so no knob name can tell it apart.
	if fam, ok := activeFamilyStage(schema); ok && fam.Percussive {
		return macroGroupDefsPercussion
	}
	return macroGroupDefsMelodic
}

// macroGroupLabel / macroGroupGloss resolve a table entry's localized strings.
func macroGroupLabel(def macroGroupDef) string { return i18n.T(def.LabelKey) }
func macroGroupGloss(def macroGroupDef) string { return i18n.T(def.GlossKey) }

// buildMacroGroups computes the in-use concept buckets for one instrument from
// its document's knob schema + current param values. Pure — no DrumView state —
// so it is unit-testable against any schema/value pair. Never returns an empty
// list for a non-empty schema: HIDE-UNUSED is skipped for the last surviving
// concept so the root always has something to tap.
func buildMacroGroups(schema []audio.ParamDef, merged audio.RecipeParams) []macroGroup {
	return buildMacroGroupsFiltered(schema, merged, true, 0)
}

// buildMacroGroupsForVoice is buildMacroGroups for a voice that sounds for
// voiceSecs seconds (the root's dials then hide envelope changes that land
// past the end of the note — see the "settles" dial condition).
func buildMacroGroupsForVoice(schema []audio.ParamDef, merged audio.RecipeParams, voiceSecs float64) []macroGroup {
	return buildMacroGroupsFiltered(schema, merged, true, voiceSecs)
}

// buildMacroGroupsAll is buildMacroGroups WITHOUT the HIDE-UNUSED filter: every
// concept that owns at least one visible param, active or not. The coverage
// discipline test uses it (an off LFO's params are still claimed by Movement —
// they are reached through the pipeline strip's stage drill, not the root).
func buildMacroGroupsAll(schema []audio.ParamDef, merged audio.RecipeParams) []macroGroup {
	return buildMacroGroupsFiltered(schema, merged, false, 0)
}

func buildMacroGroupsFiltered(schema []audio.ParamDef, merged audio.RecipeParams, hideUnused bool, voiceSecs float64) []macroGroup {
	if len(schema) == 0 {
		return nil
	}
	family, familyOK := activeFamilyStage(schema)
	defs := synthCategoryTableFor(schema)
	gates := synthStageGates(schema)
	present := make(map[string]audio.ParamDef, len(schema))
	for _, d := range schema {
		present[d.Name] = d
	}
	var out []macroGroup
	var fallback *macroGroup // first concept with params, kept if HIDE-UNUSED empties the list
	for _, def := range defs {
		var ps, ess []string
		// The gen-slot family knobs (kick/snare/tom/cymbal/FM) are claimed by
		// the REGISTRY, not by def.Params: modularStageKnobs records the card
		// each one belongs to, so surfacing a new family needs no edit here.
		// They used to be enumerated by hand as well, which is how a knob came
		// to be listed in one card and declared in another.
		//
		// On a family instrument those knobs ARE the voice, so they lead the card;
		// the generic modular stages the table also lists (OSC, FM, unison,
		// formant, LFO) ship switched off on such recipes and come after. Listed
		// generic-first, the snare's Source card opened on Wave / Octave /
		// Algorithm — stages that make no sound — with its Generator 23rd.
		gen := audio.GenSlotParamsForMacro(macroConceptFor(def.ID))
		names := append(append([]string(nil), def.Params...), gen...)
		if familyOK {
			names = append(append([]string(nil), gen...), def.Params...)
		}
		for _, name := range names {
			d, ok := present[name]
			if !ok {
				continue
			}
			if isSynthEnableParam(name) || synthParamHiddenInContext(d, family, familyOK, merged) {
				continue
			}
			ps = append(ps, name)
			if d.Tier != audio.TierAdvanced {
				ess = append(ess, name)
			}
		}
		if len(ps) == 0 {
			continue
		}
		if len(ess) == 0 {
			// Every param is fine-tuning: promote the first so the drilled view
			// never opens on an empty essentials grid.
			ess = ps[:1]
		}
		g := macroGroup{
			ID: def.ID, Label: macroGroupLabel(def), Gloss: macroGroupGloss(def),
			Stages: def.Stages, Params: ps, Essentials: ess, spriteID: "cat:" + def.ID,
			Dials: resolveCardDials(def.ID, ps, present, merged, gates, voiceSecs),
		}
		if fallback == nil {
			cp := g
			fallback = &cp
		}
		if hideUnused && def.active != nil && !def.active(merged) {
			continue // HIDE-UNUSED: the instrument doesn't use this concept
		}
		out = append(out, g)
	}
	if len(out) == 0 && fallback != nil {
		out = append(out, *fallback)
	}
	return out
}

// productionMacroCards is the production root's card list (spec 2026-10-24
// §4.2): the cards with at least one live dial, in table order. A dial-less
// card holds only discrete character choices (oscillator, generator, variant)
// and tuning — developer territory — and the production UI has nothing else to
// offer on it. Returns cards itself when nothing is dropped (no allocation on
// the common path). Guard: TestEveryInstrumentKeepsProductionDials.
func productionMacroCards(cards []macroGroup) []macroGroup {
	keep := 0
	for _, c := range cards {
		if len(c.Dials) > 0 {
			keep++
		}
	}
	if keep == len(cards) {
		return cards
	}
	out := make([]macroGroup, 0, keep)
	for _, c := range cards {
		if len(c.Dials) > 0 {
			out = append(out, c)
		}
	}
	return out
}

// macroConceptFor maps a macro CARD id back to the concept name the family-stage
// registry uses for it.
//
// The two category tables deliberately name the same ideas differently for
// their audience — a beatmaker reaches for "Source", "Boom", "Tone" and
// "Punch"; a melodic player for "Sound", "Body", "Brightness" and "Attack" —
// so one registry concept resolves to a different card depending on which
// table is active. The registry writes the percussion spelling (it was built
// for the drum families), and this is the one place that knows the melodic
// synonyms. A card with no registry concept simply claims no gen-slot knobs.
func macroConceptFor(cardID string) string {
	switch cardID {
	case "sound":
		return "source"
	case "body":
		return "boom"
	case "brightness":
		return "tone"
	case "attack":
		return "punch"
	case "warmth":
		return "grit"
	}
	return cardID
}

// nonZeroMacro is nonZero over every SLOT-1 gen-bank knob the family-stage
// registry files under a macro concept. It keeps a HIDE-UNUSED predicate
// honest across a family conversion: the concept's wired param names change,
// but the concept does not. Without it an FM instrument with a live pitch
// sweep stopped showing Pitch Drop at the root the moment it became a seed,
// because the predicate still named only the wired fm_pitch_env_amount.
func nonZeroMacro(concept string) func(audio.RecipeParams) bool {
	names := audio.GenSlotParamsForMacro(concept)
	return func(m audio.RecipeParams) bool {
		for _, n := range names {
			if v, ok := m[n]; ok && v != 0 {
				return true
			}
		}
		return false
	}
}

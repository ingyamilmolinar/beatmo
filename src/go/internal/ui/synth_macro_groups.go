package ui

// A UI-only category layer over the raw params: no persisted state, no audio-path change. Every schema gets
// categories; each concept is ONE idea; pickers stay pickers; a concept shows only when the instrument uses it.

import (
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
	"github.com/ingyamilmolinar/beatmo/internal/synthdial"
)

type macroGroup struct {
	ID     string
	Label  string
	Gloss  string
	Stages []synthSectionID
	Params []string // child param names, in display order
	// The non-Advanced params in table order; the rest sit under More.
	Essentials []string
	// spriteID keys the root card's render cache ("cat:" + ID), built once.
	spriteID string
	// Each term is a param this card claims.
	Dials []synthdial.Resolved
}

// Params lists everything the concept CAN own across instruments; active decides HIDE-UNUSED (nil = always).
// A concept with exactly one stage shows that stage's pill when drilled.
type macroGroupDef struct {
	ID       string
	LabelKey i18n.Key
	GlossKey i18n.Key
	Stages   []synthSectionID
	Params   []string
	active   func(m audio.RecipeParams) bool
}

func enableOn(name string) func(audio.RecipeParams) bool {
	return func(m audio.RecipeParams) bool { return m[name] >= 0.5 }
}

// Width and Human Feel mean nothing with a single voice.
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

// Ordered by what a beginner reaches for first. Every visible param except `gain` (the row volume) is claimed
// by exactly one concept.
var macroGroupDefsMelodic = []macroGroupDef{
	{ID: "sound", LabelKey: i18n.KeySynthCatSound, GlossKey: i18n.KeySynthCatSoundGloss,
		Stages: []synthSectionID{synthSectionOsc, synthSectionFM, synthSectionVoice, synthSectionFinish},
		Params: []string{"osc_type", "osc_octave", "osc_detune", "pitch", "post_pitch",
			"fm_algorithm", "fm_op1_ratio", "fm_op2_ratio", "fm_op3_ratio", "fm_op4_ratio",
			"fm_op1_depth", "fm_op2_depth", "fm_op3_depth", "fm_op4_depth",
			"fm_op1_level", "fm_op2_level", "fm_op3_level", "fm_op4_level",
			"osc_sax_blow", "osc_sax_reed_off", "osc_sax_reed_slope", "osc_sax_reflect", "osc_sax_breath", "osc_sax_loss",
			"osc_bow_pos", "osc_bow_slope", "osc_bow_vel", "osc_bow_loss"}},
	// The WAV-clone stages: a measured instrument's modes, its residual noise and the mallet.
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
		// nonZeroMacro covers the gen-bank spellings, so a family converted to a config seed keeps lighting this card.
		active: anyOf(enableOn("pitchenv_enabled"), nonZeroMacro("pitchdrop"))},
	{ID: "voice", LabelKey: i18n.KeySynthCatVoice, GlossKey: i18n.KeySynthCatVoiceGloss,
		Stages: []synthSectionID{synthSectionFormant},
		Params: []string{"formant_vowel", "formant_voice_type", "formant_mix", "formant_breath", "formant_sing",
			"formant_shift", "formant_morph_rate", "formant_morph_to", "formant_dry"}, active: enableOn("formant_enabled")},
	{ID: "flam", LabelKey: i18n.KeySynthCatFlam, GlossKey: i18n.KeySynthCatFlamGloss,
		Stages: []synthSectionID{synthSectionBurst},
		Params: []string{"burst_sharp", "burst1_off", "burst1_amp", "burst2_off", "burst2_amp", "burst3_off", "burst3_amp", "burst4_off", "burst4_amp"}, active: enableOn("burst_enabled")},
}

// Ordered by what a beatmaker reaches for first: transient → body → pitch → tone → tail → engine → grit.
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

func synthCategoryTableFor(schema []audio.ParamDef) []macroGroupDef {
	// A drum is identified by its gen-slot source: its knobs are gen1_<family>_*, so no knob name tells it apart.
	if fam, ok := activeFamilyStage(schema); ok && fam.Percussive {
		return macroGroupDefsPercussion
	}
	return macroGroupDefsMelodic
}

// macroGroupLabel / macroGroupGloss resolve a table entry's localized strings.
func macroGroupLabel(def macroGroupDef) string { return i18n.T(def.LabelKey) }
func macroGroupGloss(def macroGroupDef) string { return i18n.T(def.GlossKey) }

// Pure, so it is testable against any schema/value pair. Never empty for a non-empty schema: the last concept
// skips HIDE-UNUSED so the root always has something to tap.
func buildMacroGroups(schema []audio.ParamDef, merged audio.RecipeParams) []macroGroup {
	return buildMacroGroupsFiltered(schema, merged, true, 0)
}

// The root's dials then hide envelope changes that land past the end of the note.
func buildMacroGroupsForVoice(schema []audio.ParamDef, merged audio.RecipeParams, voiceSecs float64) []macroGroup {
	return buildMacroGroupsFiltered(schema, merged, true, voiceSecs)
}

// Without HIDE-UNUSED, for the coverage test: an off LFO's params are still claimed by Movement.
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
		// The gen-slot family knobs are claimed by the REGISTRY, not def.Params, so a new family needs no edit here;
		// on a family instrument they ARE the voice and lead the card, ahead of the generic stages it ships switched off.
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
			// All fine-tuning: promote the first so the drilled view never opens on an empty grid.
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

// A dial-less card holds only pickers and tuning, which production has nothing to offer for. Returns cards
// itself when nothing is dropped, so the common path allocates nothing.
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

// The two tables name the same ideas for their audience ("Source"/"Punch" vs "Sound"/"Attack"); the registry
// uses the percussion spelling and this is the one place that knows the melodic synonyms.
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

// Keeps HIDE-UNUSED honest across a family conversion: the wired param names change, the concept does not.
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

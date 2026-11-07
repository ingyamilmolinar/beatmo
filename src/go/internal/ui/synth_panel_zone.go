package ui

import (
	"fmt"
	"image"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// Synth-tab implementation — the canonical per-instrument pipeline
// surface: header strip → unified section cards (VOICE, OSC, FM,
// ENVELOPE, FILTER, POST — identical order for every synth instrument,
// empty sections pruned; every stage section carries an enable pill) →
// OUT-column launchers (FX, EQ, Delay, Reverb) → reset footer. Knobs
// come from the recipe's ParamDefs (family/voice knobs + the appended
// modular stage params — see WiredParamsForRecipe and
// modularStageParamDefs); per-stage `*_enabled` toggles render as pills,
// and a disabled stage dims its knobs under a scrim while staying
// interactive (modular-synth-unification Phase 8B).
//
// Why a zone-shaped renderer rather than a portal overlay: the audio
// panel is the canonical place to inspect & adjust an instrument's
// signal chain. A dedicated tab places synth params on the same surface
// users already consult for EQ/Wave/Spectrum/Levels/Chain. The portal
// alternative would re-introduce a stacked-modal UX that the user has
// explicitly rejected (see [[feedback_minimal_ux_changes]]).
//
// Active-row resolution: same as today — `synthTabActiveInstrument()`
// resolves the EQ panel's channel pill (Master falls through to the
// first non-empty row). The header strip renders the resolved name so
// the user always knows which row is being edited; opening the chevron
// reuses the row-picker chrome to switch rows without leaving the tab.

// ---- Section model -----------------------------------------------------

type synthSectionID int

const (
	synthSectionPitch synthSectionID = iota
	synthSectionEnvelope
	synthSectionTone
	synthSectionDrive
	// Modular-voice stages — rendered for every synth instrument (Phase 8B
	// unification). The standardized pipeline is VOICE · OSC · FM · ENVELOPE ·
	// FILTER · POST. synthSectionDrive is the canonical POST section (carries
	// the generic post knobs + gain); it keeps its old name for
	// source compatibility but labels as "POST".
	synthSectionOsc
	synthSectionFM
	synthSectionFilter
	// synthSectionVoice hosts the per-instrument family/voice knobs — the
	// knobs that ARE the sound (kick_*, snare_*, cym_*, bass_*, the FM voice's
	// fm_* family operators on fm-* recipes, the <family>_wave generator
	// picker, and fundamental). The modular voice has no family knobs, so its
	// VOICE section is empty and PRUNED (nothing for the user to do there). When
	// VOICE is populated it carries a mandatory, greyed, locked-ON pill — the
	// voice is the instrument and can't be disabled (synthStageEnableState).
	synthSectionVoice
	// Phase-8C modulator stages (the spec-§1 gap closure): PITCH ENV reuses
	// synthSectionPitch (same label/gloss); LFO and BURST get their own IDs.
	// All three default DISABLED on every recipe (new DSP; off = the exact
	// pre-Phase-8C render), with audible knob defaults behind the pill.
	synthSectionLFO
	synthSectionBurst
	// synthSectionFilterEnv is the filter ENVELOPE stage (filtenv_amt/decay +
	// filtenv_enabled pill, group "filtenv"). It is its OWN section, distinct from
	// the static FILTER stage, so the FILTER pill stays filter_enabled (the rest
	// of the UI keys off it) and the env's own enable gate gets a real pill rather
	// than clobbering FILTER's. Appended after FILTER in pipeline order.
	synthSectionFilterEnv
	// synthSectionKick is the configurable KICK stage: the modular gen-bank kick
	// voice (source==5) surfaced as an editable stage (group "kick" knobs +
	// kick_enabled pill). Shown only for modular kick instruments (e.g. dnb-kick);
	// the legacy drum-kick recipes keep their kick_* knobs in VOICE (group "core").
	synthSectionKick
	// Phase-15 voice/choir stages (Task 8). ENSEMBLE hosts the unison knobs
	// (moved out of OSC) + the humanized-vibrato knobs; its off-state is a VALUE
	// (voices==1) not a *_enabled param, so its pill is value-gated
	// (synthStageEnableState drives unison_voices). FORMANT is the vowel bank
	// (formant_enabled pill). RESONATOR surfaces the body-resonator bank; its
	// pill is likewise value-gated on body_model ("Off"==0).
	synthSectionEnsemble
	synthSectionFormant
	synthSectionResonator
	// synthSectionFinish is the FINISH stage: the post ops that run after the
	// voice's drive/gain (resample, fade, low boost, air, darken, crunch) and the
	// output limit (group "finish"). Always on — each op is a no-op at its
	// identity — so it has no enable pill.
	synthSectionFinish
	// The WAV-clone stages (spec 2026-10-12-wav-clone-modal-design §3f):
	// MODAL (the measured mode table's macros, modal_enabled pill), STRIKE (the
	// mallet, strike_enabled) and NOISE (the residual bands, noiseband_enabled).
	// Present only on documents that declare the stages. Appended so no
	// existing section id is renumbered.
	synthSectionModal
	synthSectionStrike
	synthSectionNoise
)

// synthSectionPost is an alias for the DRIVE section, which is the canonical
// POST stage in the unified model. Using a named alias keeps the routing
// table readable ("→ POST") without churning the underlying iota.
const synthSectionPost = synthSectionDrive

// unifiedSynthSectionOrder is the canonical, STANDARDIZED pipeline order shown
// for EVERY synth instrument (Phase 8B unification + the Phase-8C modulator
// stages): the per-instrument voice knobs, then the standardized stages each
// with its own enable pill. This is the deliverable the modular-synth-
// unification project targeted — the user sees the same sections, in the same
// order, for every synth sound, and can enable/disable any stage on any
// instrument.
//
//	VOICE · OSC · ENSEMBLE · FM · PITCH · LFO · BURST · ENVELOPE · FILTER ·
//	FILTER ENV · FORMANT · RESONATOR · POST · FINISH
//
// FINISH (the always-on post ops + output limit) has no enable pill: each op
// is a no-op at its identity.
//
// Sections with no content for a given recipe are pruned by
// synthSectionOrderForSchema:
//   - VOICE is omitted when the recipe has no family/voice knobs (the modular
//     voice — its OSC stage IS the sound, no separate family knobs).
//   - FM is omitted when the recipe neither appends the FM stage nor routes any
//     family knob into FM (fm-* recipes route their fm_* operators to VOICE,
//     since the FM voice IS the instrument, and the FM stage is collision-
//     excluded — so they show no separate FM stage card).
//   - PITCH/LFO/BURST (Phase-8C modulators) are appended to every synth recipe
//     (their names never collide with family knobs), so they always survive.
//   - ENSEMBLE/FORMANT/RESONATOR (Phase-15 voice/choir, Task 8) are likewise
//     appended to every synth recipe (their names never collide), so they
//     always survive too.
var unifiedSynthSectionOrder = []synthSectionID{
	synthSectionVoice,
	synthSectionOsc,
	synthSectionModal,
	synthSectionStrike,
	synthSectionNoise,
	synthSectionEnsemble,
	synthSectionFM,
	synthSectionPitch,
	synthSectionLFO,
	synthSectionBurst,
	synthSectionEnvelope,
	synthSectionFilter,
	synthSectionFilterEnv,
	synthSectionFormant,
	synthSectionResonator,
	synthSectionPost,
	synthSectionFinish,
}

// synthSectionOrderForSchema returns the standardized section order for the
// given recipe schema, pruning empty sections. Every synth instrument is laid
// out with the same VOICE·OSC·FM·ENVELOPE·FILTER·POST sequence; only sections
// that have at least one knob or an enable pill survive. POST always survives
// (every recipe carries gain, or a generic post knob).
func synthSectionOrderForSchema(schema []audio.ParamDef) []synthSectionID {
	vals := schemaValues(schema)
	// Tally per-section content: a knob (grid param) or an enable pill (a
	// per-stage *_enabled toggle that is actually appended to this recipe).
	hasContent := map[synthSectionID]bool{}
	for _, d := range schema {
		if d.Group == audio.SynthHiddenGroup || !audio.FlatFieldDeclared(vals, d.Name) {
			continue
		}
		// Any non-hidden param present in the schema — a knob OR an enable
		// toggle — makes its standardized section non-empty. Collision-excluded
		// stage params are simply absent from the schema (the recipe never
		// appended them), so they can't conjure an empty stage card here.
		hasContent[sectionForParamDef(d)] = true
	}
	out := make([]synthSectionID, 0, len(unifiedSynthSectionOrder))
	for _, sec := range unifiedSynthSectionOrder {
		if hasContent[sec] {
			out = append(out, sec)
		}
	}
	return out
}

// sectionLabel returns the display label for the section header.
func sectionLabel(id synthSectionID) string {
	switch id {
	case synthSectionPitch:
		return "PITCH"
	case synthSectionEnvelope:
		return "ENVELOPE"
	case synthSectionTone:
		return "TONE"
	case synthSectionPost: // == synthSectionDrive
		return "POST"
	case synthSectionOsc:
		return "OSC"
	case synthSectionFM:
		return "FM"
	case synthSectionFilter:
		return "FILTER"
	case synthSectionFilterEnv:
		return "FILTER ENV"
	case synthSectionVoice:
		return "VOICE"
	case synthSectionLFO:
		return "LFO"
	case synthSectionBurst:
		return "BURST"
	case synthSectionKick:
		return "KICK"
	case synthSectionEnsemble:
		return "ENSEMBLE"
	case synthSectionFormant:
		return "FORMANT"
	case synthSectionResonator:
		return "RESONATOR"
	case synthSectionFinish:
		return "FINISH"
	case synthSectionModal:
		return "MODAL"
	case synthSectionStrike:
		return "STRIKE"
	case synthSectionNoise:
		return "NOISE"
	}
	return ""
}

// sectionLabelKey maps a section to its display-label i18n key. Returns "" for
// an unknown section.
func sectionLabelKey(id synthSectionID) i18n.Key {
	switch id {
	case synthSectionPitch:
		return i18n.KeySynthStagePitch
	case synthSectionEnvelope:
		return i18n.KeySynthStageEnvelope
	case synthSectionTone:
		return i18n.KeySynthStageTone
	case synthSectionPost: // == synthSectionDrive
		return i18n.KeySynthStagePost
	case synthSectionOsc:
		return i18n.KeySynthStageOsc
	case synthSectionFM:
		return i18n.KeySynthStageFM
	case synthSectionFilter:
		return i18n.KeySynthStageFilter
	case synthSectionFilterEnv:
		return i18n.KeySynthStageFilterEnv
	case synthSectionVoice:
		return i18n.KeySynthStageVoice
	case synthSectionLFO:
		return i18n.KeySynthStageLFO
	case synthSectionBurst:
		return i18n.KeySynthStageBurst
	case synthSectionEnsemble:
		return i18n.KeySynthStageEnsemble
	case synthSectionFormant:
		return i18n.KeySynthStageFormant
	case synthSectionResonator:
		return i18n.KeySynthStageResonator
	case synthSectionFinish:
		return i18n.KeySynthStageFinish
	case synthSectionModal:
		return i18n.KeySynthStageModal
	case synthSectionStrike:
		return i18n.KeySynthStageStrike
	case synthSectionNoise:
		return i18n.KeySynthStageNoise
	}
	return ""
}

// sectionShortLabelKey maps a stage to its designed SHORT chip label ("ENV",
// "F.ENV"), or "" when the full label is already compact.
func sectionShortLabelKey(id synthSectionID) i18n.Key {
	switch id {
	case synthSectionEnvelope:
		return i18n.KeySynthStageEnvelopeShort
	case synthSectionFilterEnv:
		return i18n.KeySynthStageFilterEnvShort
	case synthSectionEnsemble:
		return i18n.KeySynthStageEnsembleShort
	case synthSectionFormant:
		return i18n.KeySynthStageFormantShort
	case synthSectionResonator:
		return i18n.KeySynthStageResonatorShort
	}
	return ""
}

// sectionLabelChipFitted returns the chip label fitted to availPx (measured
// with the styled RoleCaption metric the chip draws with): the full localized
// label when it fits, else the stage's designed short label, else ellipsis
// truncation as the last resort. Mid-word ellipses ("ENVELOP…") read worse
// than a designed abbreviation on the wrapped mobile chip strip.
func sectionLabelChipFitted(id synthSectionID, availPx int) string {
	full := sectionLabelLocalized(id)
	if StyledTextWidth(full, RoleCaption) <= availPx {
		return full
	}
	if k := sectionShortLabelKey(id); k != "" {
		if s := i18n.T(k); StyledTextWidth(s, RoleCaption) <= availPx {
			return s
		}
	}
	return truncCaptionStyled(full, availPx, RoleCaption)
}

// sectionLabelLocalized is the DISPLAY label for a stage chip/title, resolved
// through the active locale. The canonical sectionLabel stays English and backs
// hit-area tags + the selectSynthSection JS lookup — those must not shift when
// the language changes, so the two functions are deliberately separate. In the
// English locale the i18n catalog mirrors the canonical strings, so this returns
// exactly sectionLabel's value (zero behavior change for the default locale).
func sectionLabelLocalized(id synthSectionID) string {
	k := sectionLabelKey(id)
	if k == "" {
		return sectionLabel(id)
	}
	return i18n.T(k)
}

// sectionSubtitleKey maps a section to its kid-friendly subtitle i18n key.
// Returns "" for an unknown section (sectionSubtitle then renders nothing).
func sectionSubtitleKey(id synthSectionID) i18n.Key {
	switch id {
	case synthSectionPitch:
		return i18n.KeySynthStagePitchSub
	case synthSectionEnvelope:
		return i18n.KeySynthStageEnvelopeSub
	case synthSectionTone:
		return i18n.KeySynthStageToneSub
	case synthSectionPost: // == synthSectionDrive
		return i18n.KeySynthStagePostSub
	case synthSectionOsc:
		return i18n.KeySynthStageOscSub
	case synthSectionFM:
		return i18n.KeySynthStageFMSub
	case synthSectionFilter:
		return i18n.KeySynthStageFilterSub
	case synthSectionFilterEnv:
		return i18n.KeySynthStageFilterEnvSub
	case synthSectionVoice:
		return i18n.KeySynthStageVoiceSub
	case synthSectionLFO:
		return i18n.KeySynthStageLFOSub
	case synthSectionBurst:
		return i18n.KeySynthStageBurstSub
	case synthSectionEnsemble:
		return i18n.KeySynthStageEnsembleSub
	case synthSectionFormant:
		return i18n.KeySynthStageFormantSub
	case synthSectionResonator:
		return i18n.KeySynthStageResonatorSub
	case synthSectionFinish:
		return i18n.KeySynthStageFinishSub
	case synthSectionModal:
		return i18n.KeySynthStageModalSub
	case synthSectionStrike:
		return i18n.KeySynthStageStrikeSub
	case synthSectionNoise:
		return i18n.KeySynthStageNoiseSub
	}
	return ""
}

// sectionSubtitle returns a kid-friendly secondary label rendered under
// the jargon section title (Phase 4 / 5 of the audio-panel redesign).
// Always-on — both labels visible so the section card teaches the
// vocabulary without hiding the professional term. Localized via i18n.
func sectionSubtitle(id synthSectionID) string {
	k := sectionSubtitleKey(id)
	if k == "" {
		return ""
	}
	return i18n.T(k)
}

// enableToggleSection maps a per-stage enable toggle (osc_enabled, fm_enabled,
// env_enabled, filter_enabled, drive_enabled, …) to its stage section.
func enableToggleSection(name string) (synthSectionID, bool) {
	switch name {
	case "osc_enabled":
		return synthSectionOsc, true
	case "fm_enabled":
		return synthSectionFM, true
	case "env_enabled":
		return synthSectionEnvelope, true
	case "filter_enabled":
		return synthSectionFilter, true
	case "filtenv_enabled":
		return synthSectionFilterEnv, true
	case "drive_enabled":
		return synthSectionPost, true
	// Phase-8C modulator stages. PITCH ENV reuses the PITCH section ID.
	case "pitchenv_enabled":
		return synthSectionPitch, true
	case "lfo_enabled":
		return synthSectionLFO, true
	case "burst_enabled":
		return synthSectionBurst, true
	// The kick generator's enable → VOICE (the kick voice's stage). The kick is a
	// family voice like snare/cym, so its toggle lives with its knobs in VOICE.
	case "kick_enabled":
		return synthSectionVoice, true
	// Phase-15 FORMANT stage (Task 8).
	case "formant_enabled":
		return synthSectionFormant, true
	// WAV-clone stages.
	case "modal_enabled":
		return synthSectionModal, true
	case "strike_enabled":
		return synthSectionStrike, true
	case "noiseband_enabled":
		return synthSectionNoise, true
	}
	return 0, false
}

// modularStageGroupSection maps a stage param's canonical Group (osc/fm/env/
// filter/post, carried verbatim from ModularSynthParamDefs) to its standardized
// section. Used only for params that are genuine APPENDED stage params on the
// active recipe — a same-named family/generic knob routes via the family rules.
func modularStageGroupSection(group string) (synthSectionID, bool) {
	switch group {
	case "osc":
		return synthSectionOsc, true
	case "fm":
		return synthSectionFM, true
	case "env":
		return synthSectionEnvelope, true
	case "filter":
		return synthSectionFilter, true
	case "filtenv":
		return synthSectionFilterEnv, true
	case "post":
		return synthSectionPost, true
	// Phase-8E/F unison/ensemble + Phase-15 humanization (ens_*) get their own
	// ENSEMBLE section (Task 8) — no enable pill, voices==1 is the off-state.
	case "unison":
		return synthSectionEnsemble, true
	// Phase-15 FORMANT vowel-bank stage (Task 8).
	case "formant":
		return synthSectionFormant, true
	// Phase-15 RESONATOR body bank (Task 8) — no enable pill, body_model's
	// "Off" enum entry is the off-state.
	case "resonator":
		return synthSectionResonator, true
	case audio.FinishGroup:
		return synthSectionFinish, true
	// WAV-clone stages.
	case "modal":
		return synthSectionModal, true
	case "strike":
		return synthSectionStrike, true
	case "noiseband":
		return synthSectionNoise, true
	// Phase-8C modulator stages (Groups carried verbatim from
	// ModularSynthParamDefs). PITCH ENV reuses the PITCH section ID.
	case "pitchenv":
		return synthSectionPitch, true
	case "lfo":
		return synthSectionLFO, true
	case "burst":
		return synthSectionBurst, true
	}
	return 0, false
}

// activeFamilyStage returns the gen-slot voice family a recipe's SCHEMA drives,
// identified by the SEEDED default of gen1_source.
//
// This replaced a pair of near-identical schemaUsesKickStage /
// schemaUsesSnareStage functions (and would have grown a third, fourth and
// fifth as tom, cymbal and FM were surfaced). A config-first instrument is
// identified by its SOURCE, never by a family-knob name — its knobs are
// gen1_<family>_*, so a name-based check classifies it wrongly.
func activeFamilyStage(schema []audio.ParamDef) (audio.ModularFamilyStage, bool) {
	for _, d := range schema {
		if d.Name == "gen1_source" {
			return audio.FamilyStageForSource(d.Default)
		}
	}
	return audio.ModularFamilyStage{}, false
}

// schemaValues is a schema's flat value view: each ParamDef's Default, which
// InstrumentParamDefs fills from the instrument's document.
func schemaValues(schema []audio.ParamDef) map[string]float64 {
	out := make(map[string]float64, len(schema))
	for _, d := range schema {
		out[d.Name] = d.Default
	}
	return out
}

// sectionForParamDef is the canonical router: it routes a param into one of the
// standardized sections.
//
//   - Stage params (osc_*/fm_*/amp_*/filter_*/gain and the *_enabled toggles)
//     route by their canonical stage Group → OSC/FM/ENVELOPE/FILTER/POST.
//   - Generic post knobs (pitch/decay/tone/drive/body/brightness) → POST, EXCEPT
//     decay which is envelope-shaped → ENVELOPE.
//   - Everything else (the per-instrument family knobs — kick_*, snare_*, cym_*,
//     bass_*, the fm-* recipes' fm_* voice operators, the <family>_wave generator
//     picker, and fundamental) → VOICE. These knobs ARE the instrument's sound.
func sectionForParamDef(def audio.ParamDef) synthSectionID {
	name := def.Name
	// Per-stage enable toggles (*_enabled) route to their stage section by the
	// toggle's canonical group, regardless of "appended" status — the modular
	// voice's drive_enabled is a real POST toggle even though the DRIVE stage's
	// numeric knob is collision-excluded everywhere.
	if isSynthEnableParam(name) {
		if sec, ok := enableToggleSection(name); ok {
			return sec
		}
	}
	// Gen-bank family knobs (kick / tom / snare / cymbal / FM) are the voice's
	// own knobs, so they live in the agnostic VOICE stage — exactly like the
	// snare_*/cym_*/bass_* family knobs — rather than an instrument-specific
	// stage. synthFamilyParamHidden then shows each family's only on that
	// family's recipe. Matched by NAME: the group "fm" is shared with the
	// modular FM STAGE, whose knobs must keep routing to synthSectionFM below.
	if _, ok := audio.FamilyStageGroupOfParam(name); ok {
		return synthSectionVoice
	}
	if audio.IsStageParamName(name) {
		if sec, ok := modularStageGroupSection(def.Group); ok {
			return sec
		}
	}
	// The two un-grouped post knobs every modular voice carries → POST.
	switch name {
	case "pitch", "drive":
		return synthSectionPost
	}
	// Family/voice knobs (including fm-* recipes' fm_* operators and the
	// <family>_wave generator picker) → VOICE.
	return synthSectionVoice
}

// sectionForParam routes a param by name alone (tests and name-only callers),
// using the stage param's canonical definition when there is one.
func sectionForParam(name string) synthSectionID {
	if def, ok := audio.StageParamDefByName(name); ok {
		return sectionForParamDef(def)
	}
	return sectionForParamDef(audio.ParamDef{Name: name})
}

// isSynthEnableParam reports whether a param is a modular per-stage bypass
// toggle (rendered as a section pill, never as a knob).
func isSynthEnableParam(name string) bool {
	return strings.HasSuffix(name, "_enabled")
}

// synthParamHiddenInContext is the SINGLE source of truth for "this param has no
// UI on this recipe, in its current state". It is the exact predicate the knob
// grid builds against (see the layout switch in layoutInstEditor), factored out
// so callers that need to ask "would the user ever see this?" — the focus-graph
// sweep, section tallies — cannot drift from what the grid actually renders.
//
// Three independent reasons a param has no UI:
//   - Group == SynthHiddenGroup: engine-internal (noise_seed) — no UI at all.
//   - audio.FlatFieldDeclared: the instrument's modules do not declare it — a
//     knob of another voice family, of an oscillator model not selected, of a
//     cymbal/kick variant that does not read it (spec 2026-10-04 §6: one rule
//     derived from the module types instead of four hand-written predicates).
//   - synthVoiceFreqParamHidden: the base-pitch knob where the seed leaves it 0.
//
// merged is the instrument's current flat view (audio.MergedInstrumentParamsRO).
// family/familyOK are kept for the callers' signature; the rule reads merged.
func synthParamHiddenInContext(def audio.ParamDef, family audio.ModularFamilyStage, familyOK bool, merged audio.RecipeParams) bool {
	return def.Group == audio.SynthHiddenGroup ||
		!audio.FlatFieldDeclared(merged, def.Name) ||
		synthCymbalPlateParamHidden(def, merged) ||
		synthVoiceFreqParamHidden(def, merged)
}

// synthCymbalPlateParamHidden is the one VALUE-conditional rule the module
// types cannot express: the open hi-hat (1) and crash (5) declare the plate
// knobs but run the plate only once Plate Modes >= 1 (modular_gen_slot_cymbal),
// so until then only the Plate Modes switch itself is offered.
func synthCymbalPlateParamHidden(def audio.ParamDef, merged audio.RecipeParams) bool {
	field, ok := strings.CutPrefix(def.Name, "gen1_")
	if !ok || !audio.IsCymbalModalField(field) || field == "cym_modes" {
		return false
	}
	if v := int(merged["gen1_cym_variant"] + 0.5); v != 1 && v != 5 {
		return false
	}
	return merged["gen1_cym_modes"] < 1
}

// synthVoiceFreqParamHidden hides the base-pitch knob (voice_freq_hz) on
// recipes whose seed leaves it 0 — there the voice derives its pitch from the
// note. Gated on the RECIPE default (def.Default), never the live value, so
// the knob cannot vanish mid-edit.
func synthVoiceFreqParamHidden(def audio.ParamDef, _ audio.RecipeParams) bool {
	return def.Name == "voice_freq_hz" && def.Default <= 0
}

// synthParamIsGridControlIn reports whether a ParamDef is SELECTABLE as a knob in
// the section grid of a recipe with the given context (kick stage active?
// current oscillator?). This is the contextual form of synthParamIsGridControl and
// the one any per-recipe assertion must use: a param that is hidden in context
// has no knob to select and therefore no focus graph to be right or wrong about.
func synthParamIsGridControlIn(def audio.ParamDef, family audio.ModularFamilyStage, familyOK bool, merged audio.RecipeParams) bool {
	return !synthParamHiddenInContext(def, family, familyOK, merged) && !isSynthEnableParam(def.Name)
}

// synthParamIsGridControl is the CONTEXT-FREE form: it answers "can this param ever
// be a knob?" using only the def, and is correct for callers reasoning about a
// param's kind rather than its visibility on one recipe (e.g. the family-section
// routing tests). It deliberately ignores the contextual gates — use
// synthParamIsGridControlIn when the answer must match what a given recipe shows.
func synthParamIsGridControl(def audio.ParamDef) bool {
	return def.Group != audio.SynthHiddenGroup && !isSynthEnableParam(def.Name)
}

// synthStageEnabled reports whether the modular pipeline stage gated by
// paramName is currently enabled for instID. Absent/missing → enabled (the
// shipped default is 1.0), so the helper is safe for non-modular instruments.
// Free function (no DrumView state) so the preview pane can share it.
func synthStageEnabled(instID instrument.ID, paramName string) bool {
	if instID == "" || paramName == "" {
		return true
	}
	merged := audio.MergedInstrumentParamsRO(instID)
	v, ok := merged[paramName]
	if !ok {
		return true
	}
	return v >= 0.5
}

// synthStageEssential reports whether the stage gated by enableParam is instID's
// SOLE sound source — disabling it would silence the instrument entirely (e.g.
// the OSC stage on a pure-oscillator voice like FM Bass). The enable pill for
// such a stage is drawn greyed and made non-interactive so a user can't turn
// off the one thing that produces the sound. Every shaper stage returns false.
// The classification lives in audio (StageDisableSilences) where the generator
// model belongs; this thin wrapper resolves the instrument id consistently.
func (p *synthPanel) synthStageEssential(instID instrument.ID, enableParam string) bool {
	if enableParam == "" {
		return false
	}
	return audio.StageDisableSilences(p.dv.resolveSynthInstrument(instID), enableParam)
}

// toggleSynthStage flips a modular per-stage bypass toggle. Routes through
// the same SetInstrumentParam plumbing as a knob drag (coalesced, persisted,
// voice-cache invalidated) — the change is config-only and the NEXT trigger
// of the instrument renders with the stage flipped. No automatic audition:
// an immediate full-gain one-shot layered over the sequencer's scheduled
// voices was heard as a crackle/sharp transient during playback. The
// explicit Preview button is the only audition path.
func (p *synthPanel) toggleSynthStage(instID instrument.ID, paramName string) {
	dv := p.dv

	instID = dv.resolveSynthInstrument(instID)
	if instID == "" || paramName == "" {
		return
	}
	// A sole-source generator stage (e.g. OSC on a pure-oscillator voice) can
	// never be bypassed — its pill is greyed/non-interactive. Backstop guard so
	// NO input path (direct tap, tree hit area, scene, JS export) can silence the
	// instrument by toggling off the one stage that IS its sound.
	if audio.StageDisableSilences(instID, paramName) {
		return
	}
	next := 0.0
	if !synthStageEnabled(instID, paramName) {
		next = 1.0
	}
	audio.SetInstrumentParam(instID, paramName, next)
	dv.commitInstrumentParams(instID)
}

// synthEnableKind classifies a stage's enable pill.
type synthEnableKind int

const (
	// enableNone — the stage carries no pill (should not occur for a shown
	// stage after the unification; kept as a safe default).
	enableNone synthEnableKind = iota
	// enableBool — a real `*_enabled` boolean toggle (filter_enabled, …).
	// Interactive unless it gates a sole sound source (then greyed/locked).
	enableBool
	// enableGate — a value-gated stage whose "off" is a param VALUE, not a
	// boolean: ENSEMBLE (unison_voices=1) and RESONATOR (body_model=Off). The
	// toggle drives that value; always interactive.
	enableGate
	// enableLocked — a mandatory stage that is always on and can't be
	// disabled: VOICE (the voice IS the instrument). Greyed, non-interactive.
	enableLocked
)

// synthEnableState is the resolved enable-pill model for one section — the
// single source of truth the pill rect / draw / input / hit-area all consult,
// replacing the scattered `enableParam != ""` checks. `param`/`onVal`/`offVal`
// describe how a tap mutates state (bool: 1/0; gate: active-default/identity);
// locked and none stages write nothing.
type synthEnableState struct {
	kind        synthEnableKind
	enabled     bool
	interactive bool
	param       string
	onVal       float64
	offVal      float64
}

// synthSectionHasPill reports whether a section shows an enable pill at all —
// structural (no per-instrument state needed). A boolean `*_enabled` stage, the
// value-gated ENSEMBLE/RESONATOR, and the mandatory VOICE all show one. (An
// empty VOICE is pruned before layout, so a VOICE that reaches here has knobs.)
func synthSectionHasPill(id synthSectionID, enableParam string) bool {
	return enableParam != "" ||
		id == synthSectionVoice ||
		id == synthSectionEnsemble ||
		id == synthSectionResonator ||
		id == synthSectionFinish
}

// synthStageEnableState resolves the enable pill for sec on instID.
func (p *synthPanel) synthStageEnableState(instID instrument.ID, sec *synthSection) synthEnableState {
	if sec == nil {
		return synthEnableState{}
	}
	switch {
	case sec.id == synthSectionVoice, sec.id == synthSectionFinish:
		// The voice is the instrument — mandatory, always on, never disable-able.
		// FINISH is always on too: each op is a no-op at its own identity.
		return synthEnableState{kind: enableLocked, enabled: true, interactive: false}
	case sec.enableParam != "":
		essential := p.synthStageEssential(instID, sec.enableParam)
		return synthEnableState{
			kind:        enableBool,
			enabled:     synthStageEnabled(instID, sec.enableParam),
			interactive: !essential, // a sole sound source stays locked on
			param:       sec.enableParam,
			onVal:       1,
			offVal:      0,
		}
	case sec.id == synthSectionEnsemble:
		// unison_voices: 1 == off; >=2 == on. Active default from the shipped
		// recipe (if it ships an ensemble) else 3.
		return p.synthGateEnableState(instID, "unison_voices", 2, 1, 3)
	case sec.id == synthSectionResonator:
		// body_model: 0 (Off) == off; >=1 == on. Active default from the shipped
		// recipe (if it ships a body) else 1.
		return p.synthGateEnableState(instID, "body_model", 1, 0, 1)
	}
	return synthEnableState{kind: enableNone}
}

// synthGateEnableState builds a value-gated enable state: the stage reads as ON
// when its gate param is >= threshold; a tap writes offVal (identity) or the ON
// value — the instrument's library value when that is itself active,
// otherwise onFallback.
func (p *synthPanel) synthGateEnableState(instID instrument.ID, param string, threshold, offVal, onFallback float64) synthEnableState {
	resolved := p.dv.resolveSynthInstrument(instID)
	cur := audio.MergedInstrumentParamsRO(resolved)[param]
	onVal := onFallback
	if sd, ok := audio.LibraryFieldValue(resolved, param); ok && sd >= threshold {
		onVal = sd
	}
	return synthEnableState{
		kind:        enableGate,
		enabled:     cur >= threshold,
		interactive: true,
		param:       param,
		onVal:       onVal,
		offVal:      offVal,
	}
}

// applySynthStageToggle flips sec's enable state for instID — the single write
// path behind the detail-header pill (direct tap AND tree hit-area). Boolean
// stages flip 0/1; value-gated stages move between their identity and active
// values. Locked / non-interactive stages are a no-op, and a sole-source
// generator is refused even if reached (defence in depth).
func (p *synthPanel) applySynthStageToggle(instID instrument.ID, sec *synthSection) {
	dv := p.dv
	st := p.synthStageEnableState(instID, sec)
	if !st.interactive || st.param == "" {
		return
	}
	resolved := dv.resolveSynthInstrument(instID)
	if resolved == "" {
		return
	}
	if st.kind == enableBool && audio.StageDisableSilences(resolved, st.param) {
		return // never silence the sole sound source
	}
	next := st.offVal
	if !st.enabled {
		next = st.onVal
	}
	audio.SetInstrumentParam(resolved, st.param, next)
	dv.commitInstrumentParams(resolved)
}

// applySynthStageToggleByID resolves the live section for id and toggles it.
// The tree hit-adapter carries the id (sections are rebuilt each layout, so a
// pointer would dangle).
func (p *synthPanel) applySynthStageToggleByID(instID instrument.ID, id synthSectionID) {
	dv := p.dv
	for i := range dv.instEditorSections {
		if dv.instEditorSections[i].id == id {
			p.applySynthStageToggle(instID, &dv.instEditorSections[i])
			return
		}
	}
}

// synthSection is one card in the section row. Empty knobIdxs means the
// section has no wired knobs for the current recipe and the card is
// rendered as a collapsed placeholder.
type synthSection struct {
	id       synthSectionID
	rect     image.Rectangle
	knobIdxs []int // indices into instEditorControls / instEditorBindings
	// enableParam is the modular per-stage bypass toggle param name
	// (e.g. "osc_enabled") when the active recipe carries one for this
	// section; "" for the bespoke drum/FM recipes (no per-stage toggles).
	// Rendered as an On/Off pill in the card header — NOT as a knob.
	enableParam string
}

// setSelectedSynthSection drills into stage id for instID — the stage drill
// is the ONE way a stage opens now (pipeline pill, JS selectSynthSection,
// scenes and tests all route here).
func (p *synthPanel) setSelectedSynthSection(instID instrument.ID, id synthSectionID) {
	dv := p.dv
	instID = dv.resolveSynthInstrument(instID)
	if instID == "" {
		return
	}
	dv.setSynthStageOpen(instID, id)
}

// synthSelectedSection returns the section the drilled view is showing: the
// drilled stage, or a drilled category's single stage; nil at the root or
// when the target spans several stages.
func (p *synthPanel) synthSelectedSection() *synthSection {
	dv := p.dv
	inst := dv.resolveSynthInstrument(dv.synthTabActiveInstrument())
	if inst == "" {
		return nil
	}
	if i, ok := dv.drilledStageForPill(dv.synthDrillTarget(inst)); ok && i >= 0 && i < len(dv.instEditorSections) {
		return &dv.instEditorSections[i]
	}
	if t := dv.synthDrillTarget(inst); t.kind == drillStage {
		for i := range dv.instEditorSections {
			if dv.instEditorSections[i].id == t.stage {
				return &dv.instEditorSections[i]
			}
		}
	}
	return nil
}

// synthDetailEnablePillRect returns the drilled header's stage enable pill
// rect (empty when the drilled target shows no pill).
func (p *synthPanel) synthDetailEnablePillRect() image.Rectangle {
	return p.dv.instEditorMacroPillRect
}

// synthMacroCacheKey gates the per-Layout rebuild of the category tables.
type synthMacroCacheKey struct {
	inst      instrument.ID
	n         int
	hash      string
	voiceSecs float64 // the dials' "settles" gate depends on the note length
	// The DSP's verdict on which dials are relevant (synth_dial_relevance.go):
	// the cards rebuild once when it lands.
	relSig   uint64
	relKnown bool
}

// synthHeaderLayout records the rects for the header strip — the
// "SYNTH · <instrument>" title on the left and the four action buttons
// (Preview, Save, Save As, Reset) right-aligned, mirroring the Sampler tab's
// header (layoutSamplerHeaderButtons). No thumbnail, no recipe id.
type synthHeaderLayout struct {
	rect        image.Rectangle
	previewRect image.Rectangle
	saveAsRect  image.Rectangle
	// moreRect is the "⋯" overflow (Save As, Export / Import, Reset, Restore
	// shipped) — the same action menu the mobile sticky bar opens.
	moreRect  image.Rectangle
	instLabel instrument.ID // the instrument id (the header renders its display name)
}

// instParamBinding maps a widget index to a single ParamDef. Lives on
// DrumView state (one set of bindings per row's active recipe).
type instParamBinding struct {
	def audio.ParamDef
}

// synthSaveAsButtonTag / synthPreviewButtonTag are the sentinel button texts that identify the
// header action buttons in the instEditorBtns slice. Stored in Button.Text
// because the slice is indexed positionally and routed via the text in
// synthTabHitAreas. The leading \x00 keeps them out of any text-equality path
// that might surface them on screen.
const (
	synthSaveAsButtonTag  = "\x00synth-save-as"
	synthPreviewButtonTag = "\x00synth-preview"
)

// Header / section chrome padding — fixed because they encode the visual
// rhythm of the panel rather than the interactive control sizing.
const (
	synthSectionPaddingX = 8
	synthSectionPaddingY = 6
)

// instrumentHasSynthControls reports whether an instrument id has an
// instrument document (a synth voice). It is the single classifier for "is
// this a synth, or a WAV sample?" and MUST stay in lockstep with the no-synth
// detection in buildSynthTab (len(schema) == 0 ⇒ WAV sample, banner shown). The Synth tab selector is greyed/disabled when
// this returns false for the active instrument.
func instrumentHasSynthControls(instID instrument.ID) bool {
	if instID == "" {
		return false
	}
	// A synth instrument is one with an instrument document.
	return audio.HasInstrumentDoc(instID)
}

// activeInstrumentHasSynth reports whether the instrument the Synth tab would
// currently edit (the EQ active channel, else the first row) has synth
// controls. When false, the Synth tab selector is disabled.
func (dv *DrumView) activeInstrumentHasSynth() bool {
	return instrumentHasSynthControls(dv.synthTabActiveInstrument())
}

// masterChannelSelected reports whether the audio panel's active channel is
// the master bus. The Synth and Sampler views are blocked in this state — see
// EQPanelZone.masterChannelSelected for the rationale. Mirrors the desktop
// pill-disable so the mobile bottom-nav segments stay in lockstep.
func (dv *DrumView) masterChannelSelected() bool {
	return dv.eqPanelZone != nil && dv.eqPanelZone.masterChannelSelected()
}

// synthViewDisabled reports whether the mobile Synth bottom-nav segment should
// be greyed: Master is selected, or the active instrument has no synth.
func (dv *DrumView) synthViewDisabled() bool {
	return dv.masterChannelSelected() || !dv.activeInstrumentHasSynth()
}

// samplerViewDisabled reports whether the mobile Sampler segment should be
// greyed — currently only when the master bus is selected.
func (dv *DrumView) samplerViewDisabled() bool {
	return dv.masterChannelSelected()
}

// buildSynthTab is the layout entry point invoked by EQPanelZone when
// TabSynth is active. Computes header / section / OUT column / footer
// rects, populates instEditorControls + instEditorSliders + instEditorBindings
// with one entry per wired ParamDef, and assigns each entry to its section.
// Called every Layout so re-derivation per [[feedback_runtime_profile_derivation]]
// keeps working when the runtime profile or panel size changes.
//
// Critical: knob and slider INSTANCES are preserved across re-layouts when
// the binding sequence (recipe + param names) is unchanged. Recreating
// them every Layout would destroy in-flight drag state — the knob's
// dragging flag, pressY/pressVal latch, and the hit-area handler's
// captured pointer would all reset mid-drag, manifesting as "I drag the
// knob and nothing happens." Layout is called every frame by the
// drumview tree, so any per-frame allocation here is a correctness bug,
// not just a perf concern.
func (p *synthPanel) buildSynthTab(contentR image.Rectangle, instID instrument.ID) {
	dv := p.dv

	dv.instEditorBtns = dv.instEditorBtns[:0]
	dv.instEditorSections = dv.instEditorSections[:0]
	dv.instEditorHeader = synthHeaderLayout{}
	dv.instEditorNoSynth = false
	dv.instEditorBannerRect = image.Rectangle{}
	if contentR.Empty() {
		dv.instEditorControls = dv.instEditorControls[:0]
		dv.instEditorSliders = dv.instEditorSliders[:0]
		dv.instEditorStepBadges = dv.instEditorStepBadges[:0]
		dv.instEditorBindings = dv.instEditorBindings[:0]
		return
	}

	resolved := dv.resolveSynthInstrument(instID)
	if resolved == "" {
		dv.instEditorControls = dv.instEditorControls[:0]
		dv.instEditorSliders = dv.instEditorSliders[:0]
		dv.instEditorStepBadges = dv.instEditorStepBadges[:0]
		dv.instEditorBindings = dv.instEditorBindings[:0]
		return
	}
	// Reconcile the single armed editor to the instrument being built: the armed
	// map is per-instID but the editor pointer is shared, so a switch to a
	// different instrument (channel pill / project load / row reassignment) must
	// disarm (nil the editor) or rebind — otherwise the schema-defer guard below
	// would freeze this tab on the previous instrument's schema.
	if aidx, armed := dv.synthControlArmed(resolved); !armed {
		dv.instEditorArmedEditor = nil
	} else if e := dv.instEditorArmedEditor; e == nil || e.instID != resolved || e.idx != aidx {
		dv.instEditorArmedEditor = newSynthArmedControlEditor(dv, resolved, aidx)
	}
	// The knob set is derived from the instrument's own document (sub-project
	// 1 adapter): Default = its value, ranges widened to contain it.
	schema := audio.InstrumentParamDefs(resolved)

	// No synth recipe (or a recipe with no editable params) means this
	// instrument plays a loaded WAV sample — there are no synth controls to
	// shape. Replace the whole editor with a single explanatory banner
	// rather than a grid of empty stage cards (whose trigger-pulse borders
	// also blinked distractingly around controls the sample never uses). The
	// header / footer / preview / section grid are all skipped.
	if len(schema) == 0 {
		dv.instEditorControls = dv.instEditorControls[:0]
		dv.instEditorSliders = dv.instEditorSliders[:0]
		dv.instEditorStepBadges = dv.instEditorStepBadges[:0]
		dv.instEditorBindings = dv.instEditorBindings[:0]
		dv.instEditorSections = dv.instEditorSections[:0]
		dv.instEditorPreviewRect = image.Rectangle{}
		dv.instEditorHeader = synthHeaderLayout{}
		dv.instEditorNoSynth = true
		dv.instEditorBannerRect = contentR
		return
	}

	// READ-ONLY cached merge — buildSynthTab runs every Layout (every frame),
	// so a fresh merged map here was a per-frame ~115 KB allocation.
	genMerged := audio.MergedInstrumentParamsRO(resolved)
	deferred := false
	// Defer any schema CHANGE while an armed control's inline editor is live.
	// The armed editor caches the knob pointer and edits it BY INDEX (see
	// synth_armed_control.go): a schema swap mid-edit rebuilds the binding
	// slice, so the armed index would bind a DIFFERENT param and the
	// in-flight edit would scramble it (in WASM that silently drove foreign
	// params like gain/osc_enabled, silencing the voice until reload). Hold the
	// current knob set until the armed editor disarms; the next Layout swaps
	// cleanly.
	armedEditing := dv.instEditorArmedEditor != nil
	if armedEditing && !bindingsMatchSchema(dv.instEditorBindings, schema) {
		schema = schemaFromBindings(dv.instEditorBindings)
		deferred = true
	}
	if synthDragDebug {
		dv.sdbgBuildTab(fmt.Sprintf("inst=%s deferred=%v armedEditing=%v prevBindings=%d schemaLen=%d",
			resolved, deferred, armedEditing, len(dv.instEditorBindings), len(schema)))
	}

	pp := Profile()
	mobile := pp.IsMobile()
	var headerH int
	if mobile {
		// Mobile: no in-panel banner or action row. Identity lives in the
		// channel pill; the [Preview][Save][⋯] actions live on the always-
		// present sticky bar (see MobileActionButtons + EQPanelZone trailing
		// actions). Reserve zero header so the chip strip + knob grid own the
		// full height.
		headerH = 0
	} else {
		headerH = Profile().SynthHeaderH
		// Adaptive shrink: when the panel is short, shrink the header so
		// the section row keeps at least minSectionsH for the knobs. The
		// Save / SaveAs / Reset buttons live inside the header on desktop, so
		// there's no separate footer band to reserve — sections own all
		// vertical space below the header.
		const minSectionsH = 60
		if contentR.Dy() < headerH+minSectionsH {
			headerH = contentR.Dy() - minSectionsH
		}
		// Never below one action-button row (the Sampler's header floor): the
		// Preview / Save / Save As / Reset buttons must always exist.
		if floor := Profile().DensityValues().SynthHeaderButtonH + 2*SpaceXS; headerH < floor {
			headerH = floor
		}
		if headerH > contentR.Dy() {
			headerH = contentR.Dy()
		}
	}

	// Header strip occupies the top band; sections fill everything
	// below it down to contentR.Max.Y. Footer-button rects are placed
	// inside the header by layoutSynthHeader.
	dv.layoutSynthHeader(contentR, headerH, resolved, mobile)

	sectionsRect := image.Rect(
		contentR.Min.X,
		contentR.Min.Y+headerH,
		contentR.Max.X,
		contentR.Max.Y,
	)
	if sectionsRect.Empty() {
		return
	}
	// Phase 4 audio-panel redesign: reserve right slice for the
	// preview pane (osc + ADSR + filter). The pane only renders on
	// desktop layouts; mobile keeps the legacy full-width vertical
	// stack so knobs stay touch-friendly. The reserved rect is
	// stored on dv so drawSynthTab can paint into it after the
	// sections + knobs are drawn.
	previewW := synthPreviewWidth(sectionsRect, mobile)
	if previewW > 0 {
		dv.instEditorPreviewRect = image.Rect(
			sectionsRect.Max.X-previewW, sectionsRect.Min.Y,
			sectionsRect.Max.X, sectionsRect.Max.Y,
		)
		sectionsRect.Max.X = dv.instEditorPreviewRect.Min.X - SpaceMD
		dv.instEditorMobileFocusRect = image.Rectangle{}
	} else {
		dv.instEditorPreviewRect = image.Rectangle{}
		dv.instEditorMobileFocusRect = image.Rectangle{}
	}

	// Populate per-section knob index lists from the wired schema. Reuse
	// existing knob+slider instances when the binding sequence is
	// unchanged so in-flight drag state survives the re-layout. A binding
	// sequence is "unchanged" when len(schema) and every param name match
	// the previous bindings.
	sectionOrder := synthSectionOrderForSchema(schema)
	sections := make([]synthSection, len(sectionOrder))
	for i, id := range sectionOrder {
		sections[i] = synthSection{id: id}
	}
	reuseExisting := bindingsMatchSchema(dv.instEditorBindings, schema)
	if synthDragDebug && !reuseExisting {
		dv.sdbg("[synthdrag] buildSynthTab REBUILDING knob slice (reuseExisting=false) — live edit state will be LOST; armedEditing=%v", dv.instEditorArmedEditor != nil)
	}
	if !reuseExisting {
		dv.instEditorControls = dv.instEditorControls[:0]
		dv.instEditorSliders = dv.instEditorSliders[:0]
		dv.instEditorStepBadges = dv.instEditorStepBadges[:0]
		dv.instEditorBindings = dv.instEditorBindings[:0]
		dv.instEditorBindingsGen++ // invalidates bindingIndexByName's cache
	}
	family, familyOK := activeFamilyStage(schema)
	for i, def := range schema {
		sec := sectionForParamDef(def)
		switch {
		case synthParamHiddenInContext(def, family, familyOK, genMerged): // current params gate the model knobs
			// Engine-internal (noise_seed) — no UI; the shared kick params on a
			// non-kick recipe; and the physical-model OSC knobs unless their
			// oscillator is selected. Still allocate a widget slot below so
			// knob/binding indices stay aligned.
		case isSynthEnableParam(def.Name):
			// Per-stage bypass toggle → render as the section's enable pill.
			for j := range sections {
				if sections[j].id == sec {
					sections[j].enableParam = def.Name
					break
				}
			}
		default:
			for j := range sections {
				if sections[j].id == sec {
					sections[j].knobIdxs = append(sections[j].knobIdxs, i)
					break
				}
			}
		}
		var initial float64
		if denom := def.Max - def.Min; denom > 0 {
			// Fall back to the ParamDef default when the merged map lacks the
			// key — this happens for modular pipeline params on a re-voiced
			// bespoke instrument (the bespoke recipe doesn't declare them, so
			// they take the modular identity default, matching what the C
			// dispatch uses via newModularBlock).
			cur, ok := genMerged[def.Name]
			if !ok {
				cur = def.Default
			}
			initial = (cur - def.Min) / denom
			if initial < 0 {
				initial = 0
			} else if initial > 1 {
				initial = 1
			}
		}
		// A param whose range straddles zero (gain ±dB, detune ±cents,
		// octave/pitch ±st, pan) is bipolar: its knob arc should fill from
		// the 12-o'clock centre so +0 reads as centred, not as a partial
		// ring. Derived from the def every layout; safe to set mid-drag
		// (rendering polarity only, not the value).
		bipolar := def.Min < 0 && def.Max > 0
		zeroFrac := 0.5
		if bipolar && def.Max > def.Min {
			zeroFrac = (0 - def.Min) / (def.Max - def.Min)
		}
		sc, discrete, endless := scaleFromParamDef(def)
		if reuseExisting && i < len(dv.instEditorControls) {
			// Keep the existing knob/slider — preserve drag state,
			// pressY latch, captured-handler pointer. Only refresh the
			// Value when NO drag is in progress; mid-drag, the user's
			// finger owns the value and we must not clobber it.
			if !dv.instEditorControls[i].Capturing() {
				dv.instEditorControls[i].Value = initial
				dv.instEditorSliders[i].Value = initial
			}
			dv.instEditorControls[i].Bipolar = bipolar
			dv.instEditorControls[i].ZeroFrac = zeroFrac
			dv.instEditorControls[i].Scale = sc
			dv.instEditorControls[i].Discrete = discrete
			dv.instEditorControls[i].Endless = endless
			// Refresh StepMul from the existing badge when present.
			if endless && i < len(dv.instEditorStepBadges) && dv.instEditorStepBadges[i] != nil {
				dv.instEditorControls[i].StepMul = dv.instEditorStepBadges[i].Step()
			}
			dv.instEditorBindings[i] = instParamBinding{def: def}
			continue
		}
		dv.instEditorBindings = append(dv.instEditorBindings, instParamBinding{def: def})
		k := NewKnob(initial)
		k.Bipolar = bipolar
		k.ZeroFrac = zeroFrac
		k.Scale = sc
		k.Discrete = discrete
		k.Endless = endless
		var badge *KnobStepBadge
		if endless {
			badge = NewKnobStepBadge(def)
			if persisted, ok := dv.knobStepPref(def.Name); ok {
				badge.SetStep(persisted)
			}
			k.StepMul = badge.Step()
		}
		dv.instEditorControls = append(dv.instEditorControls, k)
		dv.instEditorStepBadges = append(dv.instEditorStepBadges, badge)
		dv.instEditorSliders = append(dv.instEditorSliders, NewSlider(initial))
	}

	// Beginner MACRO layer: melodic AND percussion (kick-stage) recipes get the
	// drill-down concept UI (synth_macro_render.go) in place of the stage chip
	// strip — buildMacroGroups picks the melodic or percussion concept table.
	// WAV/sample instruments (no recipe) are neither, so they fall through to the
	// legacy stage path below unchanged. All knob/binding slots are already built
	// above; the macro layer only re-lays-out which knobs are visible (the drilled
	// concept's).
	// Every recipe with a schema takes the category layer (WAV/sample
	// instruments returned earlier with the no-synth banner).
	// Prune stages with nothing for the user to do: no visible knobs AND no
	// enable pill. The empty modular VOICE is the case — its physical-model
	// knobs (osc_sax_*/osc_bow_*) route here but are hidden for the current
	// osc_type, so the candidate order kept VOICE while the knob-build left it
	// empty. A populated VOICE (family voices, drums) survives and gets its
	// locked pill; ENSEMBLE/RESONATOR always carry knobs so they never prune.
	kept := sections[:0]
	for _, s := range sections {
		if len(s.knobIdxs) == 0 && s.enableParam == "" {
			continue
		}
		kept = append(kept, s)
	}
	sections = kept
	// The stage model (instEditorSections) is the source of truth for the
	// pipeline strip + stage drill + enable pills, so it is stored BEFORE the
	// category layout runs.
	dv.instEditorSections = sections

	// The category tables are rebuilt only when the instrument, its schema or
	// its params change (buildMacroGroups walks the ~1100-param schema and
	// allocates the category slices — far too much for every Layout).
	voiceSecs := dv.synthVoiceSecs(resolved)
	macroKey := synthMacroCacheKey{inst: resolved, n: len(schema), hash: p.synthParamsHash(resolved), voiceSecs: voiceSecs}
	macros := dv.instEditorMacros
	// A dial drag in progress keeps the root's dial set: the turn may flip
	// another dial's liveness (Length moves amp_decay, which gates Hold), and
	// the row must not reshuffle under the finger. Re-resolved on release.
	dv.healSynthDialGesture()
	frozen := dv.synthDialActive != nil && dv.synthDialActive.inst == resolved && macros != nil &&
		dv.instEditorMacrosKey.inst == resolved
	if !frozen {
		// buildMacroGroups is costly: it runs only when the key moves, or once
		// at an instrument's first sight to list the dials the DSP measures.
		var built []macroGroup
		build := func() []macroGroup {
			if built == nil {
				built = buildMacroGroupsForVoice(schema, genMerged, voiceSecs)
			}
			return built
		}
		hidden, sig, known := dv.synthDialHidden(resolved, func() []dialProbe { return dialProbes(build(), genMerged) })
		macroKey.relSig, macroKey.relKnown = sig, known
		if macros == nil || dv.instEditorMacrosKey != macroKey {
			macros = withoutDials(build(), hidden)
			dv.instEditorMacrosKey = macroKey
		}
	}
	if macros != nil {
		dv.instEditorMacros = macros
		dv.instEditorSectionsRect = sectionsRect
		dv.buildSynthHeaderButtons(resolved)
		dv.layoutSynthMacroLayer(sectionsRect, resolved, mobile)
		dv.instEditorScrollMax = 0
		dv.instEditorScrollPx = 0
		return
	}
	dv.instEditorMacros = nil
	// Unreachable for a non-empty schema (buildMacroGroups never returns an
	// empty list for one); kept as a defensive no-op layout.
	dv.instEditorSectionsRect = sectionsRect
	dv.buildSynthHeaderButtons(resolved)
	dv.instEditorScrollMax = 0
	dv.instEditorScrollPx = 0
}

// schemaFromBindings reconstructs the ParamDef schema currently bound to the
// synth-tab knobs so a deferred Layout can keep the exact knob set live for the
// duration of an in-flight drag.
func schemaFromBindings(bindings []instParamBinding) []audio.ParamDef {
	out := make([]audio.ParamDef, len(bindings))
	for i, b := range bindings {
		out[i] = b.def
	}
	return out
}

// bindingsMatchSchema reports whether the existing bindings line up with
// the new schema by length + param name. Used by buildSynthTab to decide
// whether to reuse existing knob/slider instances (preserving live state)
// or rebuild the slice from scratch (when the active recipe changed).
func bindingsMatchSchema(bindings []instParamBinding, schema []audio.ParamDef) bool {
	if len(bindings) != len(schema) {
		return false
	}
	for i := range schema {
		if bindings[i].def.Name != schema[i].Name {
			return false
		}
	}
	return true
}

// layoutSynthHeader fills dv.instEditorHeader: the "SYNTH · <instrument>"
// title band with Preview / Save As / ⋯ right-aligned, laid out
// exactly like the Sampler header (layoutSamplerHeaderButtons) so the two
// tabs read as one family. Mobile relocates the actions to the sticky bar and
// drops the title (redundant with the highlighted nav tab + channel pill), so
// it lays out nothing in-panel.
func (p *synthPanel) layoutSynthHeader(contentR image.Rectangle, headerH int, instID instrument.ID, mobile bool) {
	dv := p.dv

	if mobile {
		dv.instEditorHeader = synthHeaderLayout{}
		dv.instEditorHeaderBtns = dv.instEditorHeaderBtns[:0]
		return
	}
	if headerH <= 0 {
		// Degenerate-short panel (test harness sizes): no band, no buttons —
		// the identity is still recorded for the callers that read it.
		dv.instEditorHeader = synthHeaderLayout{instLabel: instID}
		dv.instEditorHeaderBtns = dv.instEditorHeaderBtns[:0]
		return
	}

	r := image.Rect(contentR.Min.X, contentR.Min.Y, contentR.Max.X, contentR.Min.Y+headerH)
	dv2 := Profile().DensityValues()
	btnH := dv2.SynthHeaderButtonH
	if btnH > r.Dy() {
		btnH = r.Dy()
	}
	by := r.Min.Y + (r.Dy()-btnH)/2
	rx := r.Max.X - SpaceMD
	place := func(action string) image.Rectangle {
		w := audioTabActionButtonWidth(synthActionLabel(action))
		rr := image.Rect(rx-w, by, rx, by+btnH)
		rx -= w + SpaceSM
		return rr
	}
	moreR := image.Rect(rx-btnH, by, rx, by+btnH)
	rx -= btnH + SpaceSM
	saveAsR := place("save-as")
	previewR := place("preview")

	dv.instEditorHeader = synthHeaderLayout{
		rect:        r,
		previewRect: previewR,
		saveAsRect:  saveAsR,
		moreRect:    moreR,
		instLabel:   instID,
	}
	dv.instEditorHeaderBtns = dv.instEditorHeaderBtns[:0]
}

// ensureMobileActionButtons builds the persistent [Preview][Save As][⋯] cluster
// hosted on the sticky bar on mobile (Approach B). Built once; the buttons keep
// their OnClick behavior for the panel's lifetime while the sticky bar owns
// their per-frame layout.
func (p *synthPanel) ensureMobileActionButtons() {
	dv := p.dv
	if p.mobilePreviewBtn == nil {
		p.mobilePreviewBtn = NewSpecButton(synthActionLabel("preview"), ComponentButtonSecondary, func() {
			dv.previewActiveSynth()
		})
		p.mobilePreviewBtn.SetTextRole(RoleBody)
	}
	if p.mobileSaveAsBtn == nil {
		p.mobileSaveAsBtn = NewSpecButton(synthActionLabel("save-as"), ComponentButtonSecondary, func() {
			dv.openSaveAsDialog()
		})
		p.mobileSaveAsBtn.SetTextRole(RoleBody)
	}
	if p.mobileOverflowBtn == nil {
		p.mobileOverflowBtn = NewSpecButton("", ComponentButtonSecondary, func() {
			dv.openSynthActionMenu(p.mobileOverflowBtn.Rect())
		})
		p.mobileOverflowBtn.Icon = string(IconOverflow)
	}
}

// MobileActionButtons returns the Synth tab's trailing action cluster for the
// sticky bar ([Preview][Save As][⋯]), or nil off-mobile / when the active
// instrument has no synth controls. Refreshes localized labels and the Save
// As dirty-spec each call.
func (p *synthPanel) MobileActionButtons() []*Button {
	dv := p.dv
	if !Profile().IsMobile() {
		return nil
	}
	resolved := dv.resolveSynthInstrument(dv.synthTabActiveInstrument())
	if !instrumentHasSynthControls(resolved) {
		return nil
	}
	p.ensureMobileActionButtons()
	p.mobilePreviewBtn.Text = synthActionLabel("preview")
	p.mobileSaveAsBtn.Text = synthActionLabel("save-as")
	saveSpec := ComponentButtonSecondary
	if dv.synthSaveDirty(resolved) {
		saveSpec = ComponentButtonPrimary
	}
	p.mobileSaveAsBtn.SetSpec(saveSpec)
	return []*Button{p.mobilePreviewBtn, p.mobileSaveAsBtn, p.mobileOverflowBtn}
}

// buildSynthHeaderButtons constructs the *Button widgets that sit inside the
// header strip. Save As's spec toggles Primary↔Secondary by dirty state so the
// visual signal stays honest. Empty rects (very
// narrow panels) just skip the corresponding button entirely.
func (p *synthPanel) buildSynthHeaderButtons(resolved instrument.ID) {
	dv := p.dv

	h := dv.instEditorHeader
	if !h.previewRect.Empty() {
		previewBtn := NewSpecButton(synthPreviewButtonTag, ComponentButtonSecondary, func() {
			dv.previewActiveSynth()
		})
		previewBtn.SetTextRole(RoleBody)
		previewBtn.SetRect(h.previewRect)
		dv.instEditorBtns = append(dv.instEditorBtns, previewBtn)
	}
	if !h.saveAsRect.Empty() {
		saveSpec := ComponentButtonSecondary
		if dv.synthSaveDirty(resolved) {
			saveSpec = ComponentButtonPrimary
		}
		saveAsBtn := NewSpecButton(synthSaveAsButtonTag, saveSpec, func() {
			dv.openSaveAsDialog()
		})
		saveAsBtn.SetTextRole(RoleBody)
		saveAsBtn.SetRect(h.saveAsRect)
		dv.instEditorBtns = append(dv.instEditorBtns, saveAsBtn)
	}
	if !h.moreRect.Empty() {
		var moreBtn *Button
		moreBtn = NewSpecButton("", ComponentButtonSecondary, func() {
			dv.openSynthActionMenu(moreBtn.Rect())
		})
		moreBtn.Icon = string(IconOverflow)
		moreBtn.SetRect(h.moreRect)
		dv.instEditorBtns = append(dv.instEditorBtns, moreBtn)
	}
}

// sectionGrid returns the ONE control grid every drilled stage/category lays
// its cards into (the category layer's grid). The id is accepted for the
// callers that still address grids by stage.
func (p *synthPanel) sectionGrid(_ synthSectionID) *ControlGrid {
	dv := p.dv
	if dv.instEditorMacroGrid == nil {
		dv.instEditorMacroGrid = NewControlGrid(controlGridScrollbarStyle())
		// Continuous pixel + momentum content scroll (the stage grids' feel).
		dv.instEditorMacroGrid.SetSmoothScroll(true)
	}
	return dv.instEditorMacroGrid
}

// SynthSectionGrid exposes the control grid the drilled view lays its cards
// into (tests: column count, scroll state, visible window). One grid serves
// every stage / category; the id is accepted for callers that address it by
// stage.
func (p *synthPanel) SynthSectionGrid(id synthSectionID) *ControlGrid {
	return p.sectionGrid(id)
}

// drawSynthDetailPane draws the drilled view (the category layer's
// essentials + More card grid) — the name the armed-control and card-sprite
// tests drive the production card loop through.
func (p *synthPanel) drawSynthDetailPane(dst *ebiten.Image, instID instrument.ID) {
	p.drawSynthMacroLayer(dst, p.dv.resolveSynthInstrument(instID))
}

// synthTabUpdate ticks the per-frame scroll cooldown clock for every section
// grid (see ControlGrid.WheelStep / Tick) and pumps momentum decay for the
// smooth section grids (ControlGrid.UpdateMomentum — a no-op for any grid that
// never opted into SetSmoothScroll). Returns true when the layout should be
// re-derived (a fling is still coasting and the knob rects need to follow the
// new scroll window).
func (p *synthPanel) synthTabUpdate() bool {
	dv := p.dv

	// Advance the desktop wheel-nudge debounce; a spin that has paused for
	// wheelNudgeCommitFrames commits its single undo step here.
	dv.synthWheelNudge.tick()

	changed := false
	if g := dv.instEditorMacroGrid; g != nil {
		g.Tick()
		if g.UpdateMomentum() {
			changed = true
		}
	}
	// Arrow keyboard editing while a control is ARMED (Task 10) is NOT read
	// here: the armed control is a partial keyboard claimant, so the arrows are
	// dispatched from handleGlobalShortcuts through armedControlKeyClaimant
	// (keyboard_key_claim.go / synth_armed_control.go). That single seam is what
	// masks the same press for the grid's arrow-pan, and reading the keys here
	// too would step the value TWICE per press (isKeyJustPressed is
	// non-consuming). What stays here is the per-frame Tick that keeps the
	// kinetic barrel animating after a keypress.
	// Esc is likewise NOT handled here: handleEscape (game_input_shortcuts.go)
	// is the SINGLE authority for the Esc key and runs before this Update, so it
	// consumes Esc for the armed-control case (locking it) before falling
	// through to its Stop-playback fallback. A duplicate branch here would be
	// redundant and was the source of the Task 10 Critical bug: arming a
	// control opens no portal/focus/mode, so Esc fell through handleEscape's
	// ladder all the way to TriggerStop as an unrequested side effect.
	if instID := dv.synthTabActiveInstrument(); instID != "" {
		if _, armed := dv.synthControlArmed(instID); armed && dv.instEditorArmedEditor != nil {
			if dv.instEditorArmedEditor.Tick() {
				changed = true
			}
		}
	}
	// Advance the armed-card grow/shrink animation (independent of the barrel
	// kinetics above, and of whether a control is still "armed" — the shrink runs
	// after lock). A running animation forces a redraw + relayout each frame so
	// growArmedCard re-interpolates the reflow.
	if dv.advanceArmGrow() {
		changed = true
	}
	// The trigger glow animates until the note fades.
	if dv.glow != nil && dv.glow.tick() {
		changed = true
	}
	// A graph render landed (synth_graph_pipeline.go): draw it. While one is
	// pending keep drawing too, so the "updating" chip and the landing show
	// without waiting for an unrelated change.
	if dv.graphPipe != nil && (dv.graphPipe.dirty.Load() || dv.graphPipe.pending(slotNow)) {
		changed = true
	}
	return changed
}

// placeKnobsInSection assigns rects to every knob+slider pair in the section
// via the section's adaptive ControlGrid: >= 2 knobs per row, more when the
// card is wide enough, wrapping to multiple rows and scrolling when they
// overflow. Off-window knobs get an empty rect so they are neither drawn nor
// hit-tested (drawSynthSectionCard and synthTabHitAreas both skip empty rects).
// Each knob is centred (horizontally) inside its grid cell at the largest
// diameter that fits the cell, clamped to [SynthControlMin, SynthControlIdeal]. The
// slider's rect mirrors the knob's so legacy hit-tests + JS export rects keep
// landing on the visible widget.
// synthControlCellHeight is the per-knob grid cell height: dial + caption + gap.
// The old per-knob concept-viz band is gone (the focus graph explains the
// selected knob in the right pane), so the cell no longer reserves SynthConceptVizH.
// synthMaxGridCols caps how many card columns the synth detail grid flows. It is
// deliberately generous (the ParamCard ideal width self-limits the real column
// count on narrow panes); the cap only prevents pathological wide-pane blow-ups
// and gives the two-pass divider layout a stable upper bound for its cols probe.
const synthMaxGridCols = 12

func synthControlCellHeight() int {
	// Card-cell layout (Task 3): each grid cell is a full ParamCard slot.
	return Profile().DensityValues().ParamCardH
}

// synthSectionTierSplit partitions a stage's knob indices into Essential (always
// shown) and Advanced (collapsed under the stage's expander) by ParamDef.Tier.
func (p *synthPanel) synthSectionTierSplit(knobIdxs []int) (essential, advanced []int) {
	dv := p.dv

	for _, kIdx := range knobIdxs {
		if kIdx >= 0 && kIdx < len(dv.instEditorBindings) &&
			dv.instEditorBindings[kIdx].def.Tier == audio.TierAdvanced {
			advanced = append(advanced, kIdx)
		} else {
			essential = append(essential, kIdx)
		}
	}
	return essential, advanced
}

// shiftMoreBelowArmed pushes the More divider band and every control laid out
// below it down by however far the grown armed block now reaches past the
// divider's top, so the band never rides into the armed card. rest is the
// laid-out index list from the divider row onward (phantoms and More knobs).
func (dv *DrumView) shiftMoreBelowArmed(rest []int, armedKIdx int) {
	if armedKIdx < 0 || armedKIdx >= len(dv.instEditorControls) || dv.synthAdvDividerRect.Empty() {
		return
	}
	delta := dv.instEditorControls[armedKIdx].Rect().Max.Y + SpaceXS - dv.synthAdvDividerRect.Min.Y
	if delta <= 0 {
		return
	}
	dv.synthAdvDividerRect = dv.synthAdvDividerRect.Add(image.Pt(0, delta))
	if dv.instEditorMoreBtn != nil {
		dv.instEditorMoreBtn.SetRect(dv.synthAdvDividerRect)
	}
	for _, k := range rest {
		if k < 0 || k >= len(dv.instEditorControls) {
			continue
		}
		r := dv.instEditorControls[k].Rect()
		if r.Empty() {
			continue
		}
		r = r.Add(image.Pt(0, delta))
		dv.instEditorControls[k].SetRect(r)
		if k < len(dv.instEditorSliders) {
			dv.instEditorSliders[k].SetRect(r)
		}
	}
}

// growArmedCardInPlace is the no-reflow grow used below the More divider: the
// armed card stretches toward SynthArmedCardW × SynthArmedCardH (clamped to
// the pane) and any neighbour it now covers is hidden for the duration.
func (dv *DrumView) growArmedCardInPlace(visible []int, grid *ControlGrid, innerR image.Rectangle, armedKIdx int) {
	if armedKIdx < 0 || armedKIdx >= len(dv.instEditorControls) {
		return
	}
	k := dv.instEditorControls[armedKIdx]
	r := k.Rect()
	if r.Empty() {
		return
	}
	dv2 := Profile().DensityValues()
	if w := dv2.SynthArmedCardW; w > r.Dx() {
		r.Max.X = r.Min.X + w
	}
	if h := dv2.SynthArmedCardH; h > r.Dy() {
		r.Max.Y = r.Min.Y + h
	}
	if r.Max.X > innerR.Max.X {
		r.Max.X = innerR.Max.X
	}
	if bottom := grid.ContentRect().Max.Y; r.Max.Y > bottom {
		r.Max.Y = bottom
	}
	for _, o := range visible {
		if o < 0 || o == armedKIdx || o >= len(dv.instEditorControls) {
			continue
		}
		if dv.instEditorControls[o].Rect().Overlaps(r) {
			dv.instEditorControls[o].SetRect(image.Rectangle{})
			if o < len(dv.instEditorSliders) {
				dv.instEditorSliders[o].SetRect(image.Rectangle{})
			}
		}
	}
	k.SetRect(r)
	if armedKIdx < len(dv.instEditorSliders) {
		dv.instEditorSliders[armedKIdx].SetRect(r)
	}
}

// growArmedCard reflows the section / macro grid so the grow-animation subject
// (armedKIdx) occupies a 2×2 block and every OTHER control slides to a free cell
// (never hidden), interpolated by the eased grow progress (armGrowProgress).
// Called from both the stage detail pane and the macro drilled view. `visible` is
// the laid-out control-index slice (with -1 phantom cells for the advanced-tier
// divider band when present); the grid was already uniform-laid-out by the caller,
// which IS the resting layout. No-op once fully settled at rest (progress 0). The
// masonry is computed by reflowArmed; the block widens past 2 columns only when 2
// cells are still too narrow for the two barrel lanes (SynthArmedCardW).
func (dv *DrumView) growArmedCard(visible []int, grid *ControlGrid, innerR image.Rectangle, armedKIdx int) {
	prog := dv.armGrowProgress()
	if prog <= 0 || grid == nil {
		return
	}
	cols := grid.Cols()
	if cols < 1 {
		return
	}
	// Advanced-tier divider sections (phantom -1 cells) keep the legacy instant
	// grow: the masonry reflow's full-width divider band needs more rows than a
	// short stage detail pane has, so it would scroll off. The macro drill-down and
	// simple (no-divider) stage sections — the surfaces this animation targets —
	// have no phantoms and take the reflow path below.
	for i, k := range visible {
		if k < 0 {
			// Phantom (More divider) cells: when the armed control sits in the
			// essentials prefix, reflow just that prefix (grid positions align);
			// a fine-tuning control below the divider grows in place instead.
			for pos := 0; pos < i; pos++ {
				if visible[pos] == armedKIdx {
					dv.growArmedCard(visible[:i], grid, innerR, armedKIdx)
					dv.shiftMoreBelowArmed(visible[i:], armedKIdx)
					return
				}
			}
			dv.growArmedCardInPlace(visible, grid, innerR, armedKIdx)
			return
		}
	}
	armedPos := -1
	for i, k := range visible {
		if k == armedKIdx {
			armedPos = i
			break
		}
	}
	if armedPos < 0 {
		return
	}
	// Resting rects = the uniform layout the caller already applied. Controls read
	// their live rect; -1 phantom (divider) cells read their grid slot.
	resting := make([]image.Rectangle, len(visible))
	for i, k := range visible {
		if k >= 0 && k < len(dv.instEditorControls) {
			resting[i] = dv.instEditorControls[k].Rect()
		} else if r, ok := grid.CellRect(i); ok {
			resting[i] = r
		}
	}
	// 2×2 target, widened until the two barrel lanes fit (SynthArmedCardW).
	spanCols := 2
	if spanCols > cols {
		spanCols = cols
	}
	spanRows := 2
	wantW := Profile().DensityValues().SynthArmedCardW
	for spanCols < cols && spanCols*grid.CellW()+(spanCols-1)*grid.HGap() < wantW {
		spanCols++
	}
	cellAt := func(r, c int) image.Rectangle {
		rect, _ := grid.CellAt(r, c)
		return rect
	}
	target, rowMax := reflowArmed(visible, armedPos, cols, spanCols, spanRows, cellAt)
	grid.SetContentRows(rowMax + 1) // extend scroll so the reflow scrolls, not clips
	// A neighbour the reflow pushes onto a row BELOW the scroll window gets an
	// empty target from CellAt — a "not visible" sentinel that is also a zero rect
	// at the SCREEN ORIGIN. Lerping toward it sends the control up-and-left to
	// (0,0), shrinking to nothing: the "controls move upward into nothing" bug.
	// Re-run the (pure) reflow over UNCLIPPED lattice geometry so those neighbours
	// travel DOWN to their real destination instead; the clip below cuts them off
	// at the fold. The armed block keeps its window-aware target — spanRect's
	// drop-empty-corners rule and the height reach are tuned against it.
	offWindow := false
	for i := range target {
		if i != armedPos && target[i].Empty() {
			offWindow = true
			break
		}
	}
	if offWindow { // skipped entirely when the whole reflow lands on-window
		fullAt := func(r, c int) image.Rectangle {
			rect, _ := grid.CellAtUnclipped(r, c)
			return rect
		}
		if full, _ := reflowArmed(visible, armedPos, cols, spanCols, spanRows, fullAt); len(full) == len(target) {
			for i := range target {
				if i != armedPos && target[i].Empty() {
					target[i] = full[i]
				}
			}
		}
	}
	// Ensure the armed card reaches SynthArmedCardH (clamped to the pane bottom) so
	// the two barrel lanes have vertical room even when the stage shows a single
	// grid row and the block's lower cells are off-window — otherwise the card
	// grows in width but stays one row tall and the lanes look cramped. Mirrors the
	// legacy grow's height reach.
	if armedPos >= 0 && armedPos < len(target) && !target[armedPos].Empty() {
		wantH := Profile().DensityValues().SynthArmedCardH
		tr := target[armedPos]
		if tr.Dy() < wantH {
			tr.Max.Y = tr.Min.Y + wantH
			if bottom := grid.ContentRect().Max.Y; tr.Max.Y > bottom {
				tr.Max.Y = bottom
			}
			target[armedPos] = tr
		}
	}
	clipBottom := grid.ClipBottom()
	for i, k := range visible {
		var r image.Rectangle
		switch {
		case resting[i].Empty():
			// Off-window ALREADY at rest. lerpRect would interpolate FROM the empty
			// sentinel — i.e. out of the SCREEN ORIGIN — so the control would fly out
			// of the top-left corner as a growing square, sweeping across unrelated
			// chrome (the stray square over the preview pane). The reflow only ever
			// pushes controls DOWN, so an off-window control can never come into
			// view: it stays hidden for the whole animation.
			r = image.Rectangle{}
		default:
			r = lerpRect(resting[i], target[i], prog)
			// Cut a control travelling toward an off-window row off at the fold so it
			// slides UNDER the grid's bottom edge instead of painting over whatever
			// follows the grid. Wholly past the fold → hidden, exactly as any other
			// off-window cell. ClipBottom never touches a resting-layout rect.
			if r.Min.Y >= clipBottom {
				r = image.Rectangle{}
			} else if r.Max.Y > clipBottom {
				r.Max.Y = clipBottom
			}
		}
		if k >= 0 && k < len(dv.instEditorControls) {
			dv.instEditorControls[k].SetRect(r)
			if k < len(dv.instEditorSliders) {
				dv.instEditorSliders[k].SetRect(r)
			}
		}
	}
}

// drawSynthTab renders the prebuilt section model. Called by EQPanelZone.Draw
// when activeTab == TabSynth.
func (p *synthPanel) drawSynthTab(dst *ebiten.Image, contentR image.Rectangle, instID instrument.ID) {
	dv := p.dv

	if contentR.Empty() {
		return
	}
	resolved := dv.resolveSynthInstrument(instID)
	if resolved == "" {
		// No active row — draw a hint and return.
		DrawTextStyled(dst, i18n.T(i18n.KeyNoRowSelected), contentR.Min.X+SpaceMD, contentR.Min.Y+SpaceMD, RoleBody, TokenTextSecondary())
		return
	}

	// Sample (WAV) instrument: no synth controls to shape. Paint the single
	// explanatory banner instead of the header + section grid + footer.
	if dv.instEditorNoSynth {
		drawSynthNoSynthBanner(dst, dv.instEditorBannerRect)
		return
	}

	dv.drawSynthHeader(dst)
	// Category layer: pipeline strip + category cards at the root; drilled
	// params otherwise.
	p.drawSynthMacroLayer(dst, resolved)
	// Phase 4 audio-panel redesign: right-half preview pane (osc +
	// ADSR + filter) when the layout reserved space for it. Reads
	// the row's decay knob value via the synth bindings.
	// Right pane (desktop): at the root, ONE "Your sound" card (the full
	// rendered note); drilled, ONE signal graph for the selected control. The
	// old "Up close" steady-wave card is gone — the graph carries the detail.
	// The graph is drawn from the rendered note (synth_graph_pane.go): the
	// overview at the root, the focused dial's / selected knob's view
	// otherwise. The old pictures stay reachable as the developer picker's
	// "Legacy" view (drawSynthLegacyGraph).
	if r := dv.instEditorPreviewRect; !r.Empty() {
		p.drawSynthGraphPane(dst, r, resolved)
	}
	// Mobile: no side pane — a graph band above the controls
	// (instEditorMobileFocusRect).
	if mb := dv.instEditorMobileFocusRect; !mb.Empty() {
		p.drawSynthGraphPane(dst, mb, resolved)
	}
	dv.drawSynthFooter(dst)
	dv.drawSaveAsDialog(dst)
}

// drawSynthHeader renders the title band: the "SYNTH" group label and the
// instrument name on the left (drawAudioTabHeaderTitle); the action buttons
// are drawn by drawSynthFooter from instEditorBtns.
func (p *synthPanel) drawSynthHeader(dst *ebiten.Image) {
	dv := p.dv

	h := dv.instEditorHeader
	if h.rect.Empty() {
		return
	}
	drawRoundedRect(dst, h.rect, TokenSurface1(), fitRadius(RadiusSM, h.rect), true)
	tx := h.rect.Min.X + samplerTitlePad
	maxW := h.previewRect.Min.X - SpaceMD - tx
	if h.previewRect.Empty() {
		maxW = h.rect.Max.X - SpaceMD - tx
	}
	if maxW <= 0 {
		return
	}
	drawAudioTabHeaderTitle(dst, h.rect, tx, maxW, i18n.T(i18n.KeyCapSynthTitle), audioTabHeaderName(h.instLabel))
}

// drawSynthArmedControlCard renders the ARMED synth control's grown card
// through the shared, panel-agnostic renderer (armed_control_card.go) so the
// Synth and Sampler tabs draw an identical armed card. r is the GROWN rect
// placeKnobsInSection/growArmedCard already assigned to this control.
func (p *synthPanel) drawSynthArmedControlCard(dst *ebiten.Image, r image.Rectangle, b instParamBinding, instID instrument.ID, kIdx int) {
	dv := p.dv
	var resB barrelBinding
	if rb := (synthResolutionBinding{dv: dv, idx: kIdx, instID: instID}); rb.available() {
		resB = rb
	}
	drawArmedControlCard(dst, r, synthParamDisplayName(b.def.Name),
		dv.instEditorArmedEditor,
		synthValueBinding{dv: dv, idx: kIdx, instID: instID}, resB)
}

// No-synth banner copy. Package-level so tests can assert the exact text
// without duplicating string literals.
const (
	synthNoSynthHeadline = "This instrument does not use the synth"
	synthNoSynthSubtitle = "It plays a WAV sample — open the Sampler tab to edit it."
)

// drawSynthNoSynthBanner paints a single full-content banner explaining
// that the active instrument is a loaded sample (WAV), not a synth, so
// there are no synth controls to shape. Replaces the grid of empty section
// cards (and their distracting trigger-pulse borders) that the synth tab used
// to show for sample instruments.
func drawSynthNoSynthBanner(dst *ebiten.Image, contentR image.Rectangle) {
	if contentR.Empty() {
		return
	}
	// Card fill spanning the content area with a small inset margin so the
	// banner reads as one deliberate surface rather than the bare panel.
	cardR := contentR.Inset(SpaceMD)
	if cardR.Empty() {
		cardR = contentR
	}
	drawRoundedRect(dst, cardR, TokenSurface1(), RadiusSM, true)
	drawRoundedRect(dst, cardR, TokenBorderSubtle(), RadiusSM, false)

	cx := cardR.Min.X + cardR.Dx()/2
	availW := cardR.Dx() - 2*SpaceMD

	// Headline at panel-title role, subtitle at caption — both rune-safe
	// clipped to a narrow (mobile) panel so the hint is never dropped.
	headline := clipTextToWidthMeasure(synthNoSynthHeadline, availW,
		func(s string) int { return StyledTextWidth(s, RolePanelTitle) })
	subtitle := clipTextToWidthMeasure(synthNoSynthSubtitle, availW,
		func(s string) int { return StyledTextWidth(s, RoleCaption) })
	headlineH := StyledTextHeight(RolePanelTitle)
	subtitleH := StyledTextHeight(RoleCaption)

	// Vertically centre the headline + subtitle block.
	blockH := headlineH + SpaceSM + subtitleH
	top := cardR.Min.Y + (cardR.Dy()-blockH)/2
	if top < cardR.Min.Y+SpaceSM {
		top = cardR.Min.Y + SpaceSM
	}

	headlineW := StyledTextWidth(headline, RolePanelTitle)
	DrawTextStyled(dst, headline, cx-headlineW/2, top, RolePanelTitle, TokenTextPrimary())

	subW := StyledTextWidth(subtitle, RoleCaption)
	subY := top + headlineH + SpaceSM
	DrawTextStyled(dst, subtitle, cx-subW/2, subY, RoleCaption, TokenTextSecondary())
}

// drawSynthFooter renders the Reset / Save / Save-As buttons via the
// shared Button.Draw path. The buttons carry their NewSpecButton-assigned
// chrome (Primary/Secondary), so press/hover state, spec-driven theming,
// and visual consistency with the rest of the app come for free. Sentinel
// tags in btn.Text get translated to human labels at draw time so the
// `\x00synth-*` markers never leak to the user.
func (p *synthPanel) drawSynthFooter(dst *ebiten.Image) {
	dv := p.dv

	for _, btn := range dv.instEditorBtns {
		if btn == nil || btn.Text == synthMoreButtonTag {
			// The More divider draws its own chrome (drawSynthMoreDivider); its
			// Button is input-only.
			continue
		}
		// Translate sentinel → human label without mutating Text (the
		// sentinel is still used for hit-routing and test lookups).
		savedText := btn.Text
		if label := synthHeaderButtonLabel(btn.Text); label != "" {
			btn.Text = label
		}
		btn.Draw(dst)
		btn.Text = savedText
	}
}

// synthHeaderButtonLabel maps a header-button sentinel tag to its on-screen
// label; "" for non-sentinel texts.
func synthHeaderButtonLabel(tag string) string {
	switch tag {
	case synthPreviewButtonTag:
		return synthActionLabel("preview")
	case synthSaveAsButtonTag:
		return synthActionLabel("save-as")
	case synthBackButtonTag:
		return i18n.T(i18n.KeyMenuBack)
	}
	return ""
}

// synthActionLabel returns the localized button label for a synth header
// action id ("preview"/"save-as"). It is the single source of
// wording so the layout's width measurement and the draw path stay in sync,
// and so the wording can be shared with the Sampler tab. Labels are short
// enough that four of them fit a fixed full-width mobile action row without
// truncation in any supported language; should a future, longer translation
// not fit, abbreviate it here (one place) rather than reintroducing overflow.
func synthActionLabel(action string) string {
	switch action {
	case "preview":
		return i18n.T(i18n.KeyPreview)
	case "save-as":
		return i18n.T(i18n.KeySaveAs)
	}
	return ""
}

// synthControlCaption formats one knob's caption as "name  value[ unit]"
// with a unit-aware suffix for the params whose C transform has a known
// unit (st for semitones, % for normalised 0..1, ×N for multipliers).
// enumLabelFor returns the label for a discrete (Enum) ParamDef at the given
// value, clamping the rounded index into range. Returns "" for non-enum defs.
func enumLabelFor(def audio.ParamDef, value float64) string {
	if len(def.Enum) == 0 {
		return ""
	}
	idx := int(value + 0.5)
	if idx < 0 {
		idx = 0
	} else if idx >= len(def.Enum) {
		idx = len(def.Enum) - 1
	}
	// def.Enum is the canonical storage form; the caption is locale-aware.
	return localizeEnumLabel(def.Enum[idx])
}

func synthControlCaption(def audio.ParamDef, value float64) string {
	// Discrete (Enum) params show only the selected label — the name lives in
	// the stage title; the waveform/algorithm name IS the value.
	if lbl := enumLabelFor(def, value); lbl != "" {
		return lbl
	}
	return synthParamDisplayName(def.Name) + "  " + formatSynthParamValue(def, value)
}

// synthControlPurpose returns the one-line, kid-readable gloss for a knob ("bright
// or dull", "slight pitch shift", …) drawn under its caption. It prefers the
// param's display Label, falling back to its raw Name, and returns "" when
// neither maps (caller skips the line). Same PlainEnglish source the section
// subtitles use, applied per knob.
func synthControlPurpose(def audio.ParamDef) string {
	if p := PlainEnglish(def.Label); p != "" {
		return p
	}
	return PlainEnglish(def.Name)
}

// synthControlCaptionValueOnly returns just the value portion of the
// knob caption — the section title carries the parameter name, so on
// narrow mobile-grid cells where the full caption truncates with
// ellipsis we strip the name prefix and keep the value legible.
// Mobile-usability pass.
func synthControlCaptionValueOnly(def audio.ParamDef, value float64) string {
	if lbl := enumLabelFor(def, value); lbl != "" {
		return lbl
	}
	return formatSynthParamValue(def, value)
}

// synthControlMobileCaption is the caption band under a mobile knob's value
// pill: the parameter NAME only. The pill above already renders the value
// (knobPillLabel), so a name+value — or, for enum knobs, value-only —
// caption duplicated the pill's text ("Base"/"Base" — A9, 2026-07-04
// critique).
func synthControlMobileCaption(def audio.ParamDef) string {
	return synthParamDisplayName(def.Name)
}

// scaleFromParamDef builds the Knob's KnobScale and behavior flags from a
// ParamDef. Enum or Step>0 => discrete (clicky); otherwise endless.
func scaleFromParamDef(def audio.ParamDef) (sc KnobScale, discrete, endless bool) {
	sc = KnobScale{Min: def.Min, Max: def.Max, Unit: def.Unit, Enum: def.Enum, Step: def.Step}
	discrete = len(def.Enum) > 0 || def.Step > 0
	endless = !discrete
	return
}

// knobStepPref returns a persisted step for a param name from the global
// KnobStepSaveSink (registered via SetKnobStepSink at process start).
// Returns (0, false) when no sink is registered or no value is persisted.
func (dv *DrumView) knobStepPref(name string) (float64, bool) {
	sink := activeKnobStepSink()
	if sink == nil {
		return 0, false
	}
	steps := sink.LoadKnobSteps()
	v, ok := steps[name]
	return v, ok
}

// persistKnobStep saves the badge's currently chosen step-multiplier through
// the global KnobStepSaveSink. No-op when no sink is registered or b is nil.
func (dv *DrumView) persistKnobStep(b *KnobStepBadge) {
	if b == nil {
		return
	}
	sink := activeKnobStepSink()
	if sink == nil {
		return
	}
	_ = sink.SaveKnobStep(b.ParamName(), b.Step())
}

// handleSynthTabInput dispatches a single mouse/touch event into the
// synth tab. Returns true when the event was consumed. Called by
// EQPanelZone.handleInput when activeTab == TabSynth.
func (p *synthPanel) handleSynthTabInput(x, y int, pressed bool, instID instrument.ID) bool {
	dv := p.dv

	instID = dv.resolveSynthInstrument(instID)
	if !pressed {
		return false
	}
	// NOTE: this is a legacy DIRECT input path retained for tests + JS bridge
	// fallbacks; the production synth-tab value interaction is the tree-routed
	// ParamCard tap (synthControlHitAdapter.OnPress → value-wheel popup). There is
	// no in-place rotary drag here anymore — the knob is a headless model.
	//
	// Selected stage's enable pill (detail-pane header). Hit-test before the
	// chips/footer so a header tap toggles the stage rather than falling
	// through.
	if sel := dv.synthSelectedSection(); sel != nil && dv.synthStageEnableState(instID, sel).interactive {
		if pillR := dv.synthDetailEnablePillRect(); !pillR.Empty() && image.Pt(x, y).In(pillR) {
			dv.applySynthStageToggleByID(instID, sel.id)
			return true
		}
	}
	// Header buttons (Preview / Save As).
	for _, btn := range dv.instEditorBtns {
		if !image.Pt(x, y).In(btn.Rect()) {
			continue
		}
		switch btn.Text {
		case synthSaveAsButtonTag:
			dv.openSaveAsDialog()
			return true
		case synthPreviewButtonTag:
			dv.previewActiveSynth()
			return true
		}
	}
	return false
}

// syncSliderFromKnob copies the Knob's value into the paired Slider so the
// existing propagate/coalesce path (which reads Slider.Value) continues to
// observe the live drag value.
func (p *synthPanel) syncSliderFromKnob(idx int) {
	dv := p.dv

	if idx < 0 || idx >= len(dv.instEditorControls) || idx >= len(dv.instEditorSliders) {
		return
	}
	dv.instEditorSliders[idx].Value = dv.instEditorControls[idx].Value
}

// ---- Readout-rect helpers ----------------------------------------------

// ---- Hit areas + propagate ---------------------------------------------

// propagateSynthSliderValue rescales a 0..1 slider value into the
// ParamDef's [Min, Max] range and pushes it through audio.SetInstrumentParam.
// Coalescing matches the pre-redesign behaviour: stationary drag bursts
// don't flood the JS bridge — see
// instrument_params_playback_stability.browser.test.js.
func (p *synthPanel) propagateSynthSliderValue(idx int, instID instrument.ID) {
	dv := p.dv

	if idx < 0 || idx >= len(dv.instEditorSliders) || idx >= len(dv.instEditorBindings) {
		return
	}
	if instID == "" {
		return
	}
	sl := dv.instEditorSliders[idx]
	b := dv.instEditorBindings[idx]
	actual := b.def.Min + sl.Value*(b.def.Max-b.def.Min)
	// Discrete (Enum) params snap to the nearest integer index so the stored
	// value is an exact selector position — the C engine reads it via
	// lrintf, and Save/JSON round-trips cleanly without a fractional drift.
	if len(b.def.Enum) > 0 {
		actual = float64(int(actual + 0.5))
	}
	if synthDragDebug {
		dv.sdbg("[synthdrag] propagate WRITE inst=%s param=%s slider=%.4f actual=%.4f enum=%v", instID, b.def.Name, sl.Value, actual, len(b.def.Enum) > 0)
	}
	current := audio.GetInstrumentParams(instID)
	if prev, ok := current[b.def.Name]; ok {
		span := b.def.Max - b.def.Min
		if span <= 0 {
			span = 1
		}
		epsilon := span * 1e-4
		if abs64(prev-actual) < epsilon {
			return
		}
	}
	audio.SetInstrumentParam(instID, b.def.Name, actual)
}

// abs64 is a small inline absolute value helper.
func abs64(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// synthAuditionFn is the hook the Synth tab uses to play a one-shot of the
// edited instrument when the user EXPLICITLY asks for it via the Preview
// button. Production code points at audio.Play; tests swap it via
// SwapSynthAuditionFnForTest to assert the audition fires the expected
// number of times without producing real sound.
//
// There is no parallel "preview voice" path. Reusing audio.Play means the
// audition flows through the same trigger plumbing as a sequencer hit, so
// the effect chain, voice cache invalidation, send-FX, and mixer behave
// identically.
//
// IMPORTANT: knob releases and stage toggles must NOT call this. Param
// edits are config-only — SetInstrumentParam invalidates the voice cache
// and the NEXT trigger renders with the new values. The Phase-10 auto-
// audition-on-release fired an immediate, full-gain (vol=1.0) one-shot
// with no scheduler lead that bypassed the row/node volume; layered over
// the sequencer's scheduled voices it overshot the master chain and was
// heard as a crackle/sharp transient. Contract pinned by
// synth_no_auto_audition_test.go.
var synthAuditionFn = func(instID instrument.ID) {
	audio.Play(instID)
}

// SwapSynthAuditionFnForTest installs a test fake for the audition trigger
// and returns the previous value. Mirrors the SwapPlatformInstrumentParamsChangedForTest
// pattern in internal/audio so cross-package tests can observe the audition
// without making real audio.
func SwapSynthAuditionFnForTest(fn func(instrument.ID)) func(instrument.ID) {
	prev := synthAuditionFn
	if fn != nil {
		synthAuditionFn = fn
	}
	return prev
}

// previewActiveSynth is the Preview-button handler: a one-shot audition of
// the active instrument with the CURRENT (unsaved) knob state. The live
// overlay params are already in the audio manager (knob drags write through
// SetInstrumentParam), so a plain trigger via synthAuditionFn renders the
// current tone — letting the user preview before deciding to Save.
func (p *synthPanel) previewActiveSynth() {
	dv := p.dv

	dv.auditionInstrumentAfterDrag(dv.synthTabActiveInstrument())
	// The graph's play-through follows the audition (same sync sink a
	// sequencer hit uses).
	dv.onInstrumentTriggered(dv.resolveSynthInstrument(dv.synthTabActiveInstrument()))
}

// auditionInstrumentAfterDrag fires one audition for an explicit user
// request. Its only production caller is previewActiveSynth (the Preview
// button) — knob releases and stage toggles are config-only and must not
// route here (see synthAuditionFn). instID is the already-resolved
// instrument id; the empty string is a no-op so callers don't have to gate
// on resolveSynthInstrument returning a value.
func (p *synthPanel) auditionInstrumentAfterDrag(instID instrument.ID) {
	dv := p.dv
	_ = dv

	if instID == "" {
		return
	}
	synthAuditionFn(instID)
}

// resolveSynthInstrument resolves the editor's target row instrument id.
// Mirrors synthTabActiveInstrument but accepts a forced id (used by the
// callback bridge from EQPanelZone).
func (p *synthPanel) resolveSynthInstrument(instID instrument.ID) instrument.ID {
	dv := p.dv

	if instID != "" {
		return instID
	}
	return dv.synthTabActiveInstrument()
}

// synthTabActiveInstrument resolves "which instrument's params is the
// synth tab editing right now?" Mirrors the channel-resolution logic in
// other audio tabs: prefer the EQ panel's active channel, fall back to
// the first audible/non-empty row.
func (p *synthPanel) synthTabActiveInstrument() instrument.ID {
	dv := p.dv

	if dv.eqPanelZone != nil {
		if inst, ok := dv.eqPanelZone.ActiveChannel().Instrument(); ok {
			return inst
		}
	}
	for _, r := range dv.Rows {
		if r != nil && r.Instrument != "" {
			return r.Instrument
		}
	}
	return ""
}

// synthTabHitAreas constructs HitArea entries for every knob, footer
// button, and Save As dialog control. Z-index sits one tier above the
// EQ panel chrome; the footer + dialog get further bumps so they win on
// any geometric overlap with knobs.
func (p *synthPanel) synthTabHitAreas() []HitArea {
	dv := p.dv

	instID := dv.synthTabActiveInstrument()
	// Knobs sit two tiers above the EQ-panel chrome so the per-section scroll
	// catch-all (one tier above the chrome, below the knobs) lets a press that
	// lands on a knob still capture the knob, while a press on empty card space
	// scrolls. Pills / dialog / footer bump further so they win on overlap.
	z := ZEQPanel + 2
	// Capacity sized for the VISIBLE knob set (one expanded stage), not the
	// full schema: instEditorControls holds a slot per wired ParamDef (~1030 for
	// a modular recipe, almost all with empty rects), and reserving that many
	// HitAreas allocated ~100 KB per frame — a large slice of the Synth-tab
	// GC churn (see synth_live_param_edit_alloc_test.go). append grows the
	// slice in the rare frame where more controls are actually laid out.
	out := make([]HitArea, 0, 64)
	// Touch-min floor: expand the ParamCard's hit ClipRect so the touchable
	// area is at least TouchMinTarget on each axis even when the card's visual
	// rect is smaller (Compact-density cards can be ≤ 28 px wide). Centring the
	// expanded rect on the visual rect keeps the press point honest; the wider
	// clip only forgives near-miss taps that would otherwise fall through.
	touchMin := TouchMinTarget()
	clipFor := func(r image.Rectangle) image.Rectangle {
		clip := r.Inset(-8)
		if touchMin <= 0 {
			return clip
		}
		if w := clip.Dx(); w < touchMin {
			grow := (touchMin - w + 1) / 2
			clip.Min.X -= grow
			clip.Max.X += grow
		}
		if h := clip.Dy(); h < touchMin {
			grow := (touchMin - h + 1) / 2
			clip.Min.Y -= grow
			clip.Max.Y += grow
		}
		return clip
	}
	for i, k := range dv.instEditorControls {
		if k == nil {
			continue
		}
		r := k.Rect()
		if r.Empty() {
			continue
		}
		clip := clipFor(r)
		out = append(out, HitArea{
			Rect:     r,
			ZIndex:   z,
			Handler:  &synthControlHitAdapter{dv: dv, idx: i, instID: instID},
			Tag:      fmt.Sprintf("synth-control-%d", i),
			Touch:    true,
			ClipRect: clip,
		})
		// Also publish under the legacy "synth-slider-%d" tag so existing
		// browser tests / hit-area tests that target sliders still resolve.
		out = append(out, HitArea{
			Rect:     r,
			ZIndex:   z,
			Handler:  &synthControlHitAdapter{dv: dv, idx: i, instID: instID},
			Tag:      fmt.Sprintf("synth-slider-%d", i),
			Touch:    true,
			ClipRect: clip,
		})
		// Readout (caption) hit area — sits below the dial, outside k.Rect().
		// OnPress checks for readout/badge before starting a drag, so tapping
		// here opens the numeric editor rather than capturing a knob drag.
		// (The value caption is a simple display label now — no readout hit area,
		// so tapping the knob's title no longer opens a numeric text input.)
		// Badge hit area — sits below the caption, outside k.Rect().
		// OnPress handles the cycle; this HitArea makes the badge reachable
		// via the real tree dispatch path.
		if i < len(dv.instEditorStepBadges) {
			if b := dv.instEditorStepBadges[i]; b != nil && !b.Rect().Empty() {
				out = append(out, HitArea{
					Rect:     b.Rect(),
					ZIndex:   z + 1, // above readout so badge wins over readout on overlap
					Handler:  &synthControlHitAdapter{dv: dv, idx: i, instID: instID},
					Tag:      fmt.Sprintf("synth-badge-%d", i),
					Touch:    true,
					ClipRect: clipFor(b.Rect()),
				})
			}
		}
	}
	// Category layer: the pipeline strip + card drill handlers (root) or the
	// stage pill + grid scroll (drilled). The drilled param knobs are already
	// published by the per-knob loop above; Back / More are in instEditorBtns.
	out = append(out, p.synthMacroHitAreas(instID)...)
	// Save As dialog hit areas. Z = z+2 so the dialog floats above the
	// footer buttons.
	if dv.saveAsDialog != nil {
		dlg := dv.saveAsDialog
		out = append(out, HitArea{
			Rect:    dlg.OKRect(),
			ZIndex:  z + 2,
			Handler: &synthSaveAsDialogOKAdapter{dv: dv},
			Tag:     "synth-save-as-dialog-ok",
			Touch:   true,
		})
		out = append(out, HitArea{
			Rect:    dlg.CancelRect(),
			ZIndex:  z + 2,
			Handler: &synthSaveAsDialogCancelAdapter{dv: dv},
			Tag:     "synth-save-as-dialog-cancel",
			Touch:   true,
		})
		// Dialog frame itself consumes clicks so taps inside the dialog
		// (but outside OK/Cancel/TextInput) don't dismiss the dialog as
		// "click outside".
		out = append(out, HitArea{
			Rect:    dlg.Rect(),
			ZIndex:  z + 2 - 1, // just below OK/Cancel — so OK/Cancel win on overlap
			Handler: &synthSaveAsDialogFrameAdapter{dv: dv},
			Tag:     "synth-save-as-dialog-frame",
		})
	}
	for i, btn := range dv.instEditorBtns {
		if btn == nil {
			continue
		}
		r := btn.Rect()
		if r.Empty() {
			continue
		}
		// Every footer button dispatches through its OWN OnClick via the
		// shared buttonHitAdapter — never by sentinel text. The tag is still
		// derived from the text so existing routing tests (TestSynthFooter_*)
		// keep their per-button assertions.
		var tag string
		switch btn.Text {
		case synthSaveAsButtonTag:
			tag = "synth-save-as"
		case synthPreviewButtonTag:
			tag = "synth-preview"
		case synthBackButtonTag:
			tag = "synth-back"
		case synthMoreButtonTag:
			tag = "synth-more"
		default:
			tag = fmt.Sprintf("synth-btn-%d", i)
		}
		// z + 1: bump footer one tier above knobs / OUT-column links so
		// any click whose pixel intersects a footer rect routes to the
		// footer handler. This is the single fix for the user's bug
		// report ("Save/SaveAs/Reset are on top of other buttons that
		// trigger if clicked") — see feedback_shared_widget_reuse memory.
		out = append(out, HitArea{
			Rect:    r,
			ZIndex:  z + 1,
			Handler: &buttonHitAdapter{btn: btn},
			Tag:     tag,
			Touch:   true,
		})
	}
	return out
}

// synthToggleHitAdapter routes a per-stage enable-pill tap to toggleSynthStage.
// One-shot like the footer Save/Reset adapters (no drag); created fresh per
// Layout so it never caches a stale pointer.
type synthToggleHitAdapter struct {
	dv     *DrumView
	sec    synthSectionID
	instID instrument.ID
}

func (h *synthToggleHitAdapter) OnPress(x, y int) InputResult {
	if h.dv != nil {
		h.dv.applySynthStageToggleByID(h.instID, h.sec)
	}
	return InputCaptured
}

func (h *synthToggleHitAdapter) OnDrag(x, y int)                     {}
func (h *synthToggleHitAdapter) OnRelease(x, y int)                  {}
func (h *synthToggleHitAdapter) OnWheel(x, y, steps int) InputResult { return InputIgnored }

// synthControlHitAdapter is the per-card hit handler, routed through the
// armed-control model (see synth_armed_control.go / synth_armed_editor.go):
//   - NOT armed: a stationary tap ARMS this control (selecting it for inline
//     barrel editing); a vertical sweep scrolls the section's ControlGrid via
//     dragScrollArbiter; a wheel notch also scrolls the grid.
//   - ARMED (this control): press/drag/wheel route straight to the
//     ArmedControlEditor, spinning whichever barrel lane (value/resolution)
//     the pointer is over; a stationary tap LOCKS the control back out.
//
// The adapter does NOT cache the *Knob pointer — buildSynthTab can rebuild
// instEditorControls at any time (analyzer state changes, channel switches,
// …), which would leave a cached pointer dangling against a now-orphan
// widget. Looking up by index every call stays correct across re-layouts.
type synthControlHitAdapter struct {
	dv     *DrumView
	idx    int
	instID instrument.ID
	arb    dragScrollArbiter // tap-vs-scroll gate for the current gesture (NOT-armed path only)

	// armedMoved tracks whether the current armed gesture's CUMULATIVE pointer
	// displacement from the press point has crossed tapMaxMovePx (mirrors
	// dragScrollArbiter.drag's tap-slop test) — a stationary armed
	// press+release LOCKS, a moved one does not. This is deliberately NOT
	// "did DragTo cross a whole barrel detent": ArmedControlEditor.DragTo /
	// BarrelWheel only report true once a full SynthWheelRowH (22-32px)
	// detent is crossed, so a sub-detent drag (e.g. 12-20px, well past the
	// 10px tap slop) would otherwise look "stationary" and wrongly LOCK the
	// control instead of just spinning it partway.
	armedMoved  bool
	armedStartX int
	armedStartY int
}

// isArmed reports whether THIS control (by idx) is the currently armed one
// for its instrument.
func (h *synthControlHitAdapter) isArmed() bool {
	idx, armed := h.dv.synthControlArmed(h.instID)
	return armed && idx == h.idx
}

// ownerGrid returns the ControlGrid of the currently expanded stage — the grid
// that hosts this control and that a vertical sweep starting on it scrolls.
func (h *synthControlHitAdapter) ownerGrid() *ControlGrid {
	if h.dv == nil {
		return nil
	}
	return h.dv.instEditorMacroGrid
}

// glowRect implements glowTarget: hovering a param card lifts it through the
// shared tree hover overlay like every other clickable. The whole card is the
// hover target (a tap arms/locks it; see OnPress/OnRelease).
func (h *synthControlHitAdapter) glowRect(_, _ int, hitRect image.Rectangle) image.Rectangle {
	return hitRect
}

func (h *synthControlHitAdapter) invalidatePanel() {
	if h.dv != nil && h.dv.eqPanelZone != nil {
		h.dv.eqPanelZone.Invalidate()
	}
}

func (h *synthControlHitAdapter) OnPress(x, y int) InputResult {
	if h.isArmed() {
		// Armed: press starts a barrel drag on whichever lane (value/resolution)
		// the pointer is over. A stationary press+release LOCKS (see OnRelease).
		h.armedMoved = false
		h.armedStartX = x
		h.armedStartY = y
		if h.dv.instEditorArmedEditor != nil {
			h.dv.instEditorArmedEditor.BeginDrag(x, y)
		}
		return InputCaptured
	}
	// NOT armed: capture the gesture and begin tap-vs-scroll arbitration: a
	// vertical sweep starting on this control scrolls its stage's ControlGrid,
	// while a stationary tap ARMS the control on RELEASE. Capturing (not
	// consuming) is what lets the sweep become a scroll instead of arming on
	// the down frame — the whole point of the mobile "swipe up/down" fix.
	h.arb.begin(y, h.ownerGrid())
	return InputCaptured
}

// OnDrag advances whichever surface owns the gesture: the armed editor's
// captured barrel lane when this control is armed, otherwise the stage grid
// (tap-vs-scroll arbitration) — once travel crosses the tap slop the gesture
// becomes a scroll and the release will NOT arm.
func (h *synthControlHitAdapter) OnDrag(x, y int) {
	if h.isArmed() {
		// Cumulative displacement from the press point, not "did a whole
		// detent get crossed" — see the armedMoved doc comment.
		if abs(y-h.armedStartY) >= tapMaxMovePx {
			h.armedMoved = true
		}
		if h.dv.instEditorArmedEditor != nil && h.dv.instEditorArmedEditor.DragTo(y) {
			h.invalidatePanel()
		}
		return
	}
	if h.arb.drag(y) {
		h.invalidatePanel()
	}
}

// OnRelease finishes whichever gesture OnPress started. Armed: ends the
// barrel drag; a gesture that stayed STATIONARY (never crossed the tap slop)
// LOCKS the control back out — a click ANYWHERE on the armed card locks,
// including the value-lane readout (manual numeric text entry is disabled for
// now, 2026-07-25); a dragged spin stays armed. Not armed: ARMS the control
// only when the gesture stayed a tap (never crossed the slop) — a committed
// scroll arms nothing. Selecting the control (focus graph) happens as part of
// arming/locking so a scroll leaves no stray selection change.
func (h *synthControlHitAdapter) OnRelease(x, y int) {
	if h.isArmed() {
		if h.dv.instEditorArmedEditor != nil {
			h.dv.instEditorArmedEditor.EndDrag()
		}
		if !h.armedMoved {
			// Any stationary click anywhere on the armed card LOCKS it — including
			// the value-lane readout band (values are set with the barrel; typed
			// numeric entry was retired).
			h.dv.toggleSynthControlArm(h.instID, h.idx)
		}
		return
	}
	if !h.arb.end() {
		return
	}
	h.dv.armSynthControl(h.instID, h.idx)
}

// OnWheel routes the notch to whichever surface owns it. Armed: spins the
// hovered barrel lane (value or resolution) by one detent, coalesced into ONE
// debounced undo step by the ArmedControlEditor. Not armed: scrolls the stage
// ControlGrid — a not-armed wheel over a control is a row scroll, never a
// value edit (arm the control first).
func (h *synthControlHitAdapter) OnWheel(x, y, steps int) InputResult {
	if h.isArmed() {
		if h.dv.instEditorArmedEditor != nil && h.dv.instEditorArmedEditor.Wheel(x, y, steps) {
			h.invalidatePanel()
		}
		return InputConsumed
	}
	if g := h.ownerGrid(); g != nil && g.WheelStep(steps) {
		h.invalidatePanel()
	}
	return InputConsumed
}

// synthSectionScrollAdapter routes scroll input for one overflowing section
// card to that section's ControlGrid. With body == true it is the card-body
// catch-all (a grab on empty card space); with body == false it is the
// scrollbar thumb handle. Section grids opt into SetSmoothScroll(true), so the
// body grab and the wheel are continuous pixel scroll with momentum, while the
// thumb grab stays the row-by-row stepped drag (direct — the thumb follows the
// finger — BeginDrag); the body grab is natural (content follows the finger —
// ControlGrid.BeginContentDrag) and takes the smooth path when smooth scroll is
// on. A scroll change calls Invalidate so the next Layout re-derives the knob
// rects for the new window. Created fresh per Layout (like
// synthControlHitAdapter) so it never caches a stale grid pointer.
type synthSectionScrollAdapter struct {
	dv   *DrumView
	id   synthSectionID
	body bool
	// grid, when set, is the grid to scroll (the category layer's shared grid);
	// otherwise the adapter resolves the stage grid by id.
	grid *ControlGrid
}

func (h *synthSectionScrollAdapter) gridRef() *ControlGrid {
	if h.grid != nil {
		return h.grid
	}
	if h.dv == nil {
		return nil
	}
	return h.dv.sectionGrid(h.id)
}

func (h *synthSectionScrollAdapter) invalidate() {
	if h.dv != nil && h.dv.eqPanelZone != nil {
		h.dv.eqPanelZone.Invalidate()
	}
}

func (h *synthSectionScrollAdapter) OnPress(x, y int) InputResult {
	g := h.gridRef()
	if g == nil {
		return InputIgnored
	}
	// The thumb grab (h.body == false) starts a row-by-row stepped drag
	// anchored at the press point and follows the thumb (direct). The body
	// grab (h.body == true) follows the finger (natural) and, because section
	// grids have SetSmoothScroll(true), takes the continuous pixel + momentum
	// path instead of the stepped one.
	began := false
	if h.body {
		began = g.BeginContentDrag(y)
	} else {
		began = g.BeginDrag(y)
	}
	if began {
		return InputCaptured
	}
	return InputIgnored
}

func (h *synthSectionScrollAdapter) OnDrag(x, y int) {
	g := h.gridRef()
	if g == nil {
		return
	}
	if g.DragTo(y) {
		h.invalidate()
	}
}

func (h *synthSectionScrollAdapter) OnRelease(x, y int) {
	if g := h.gridRef(); g != nil {
		g.EndDrag()
	}
}

func (h *synthSectionScrollAdapter) OnWheel(x, y, steps int) InputResult {
	g := h.gridRef()
	if g == nil {
		return InputIgnored
	}
	// Section grids have SetSmoothScroll(true), so the wheel scrolls
	// continuously (ScrollBehavior.HandleWheel) rather than one stepped row per
	// notch. Always consume so the wheel never leaks to a lower handler while
	// the cursor is over the card.
	if g.WheelStep(steps) {
		h.invalidate()
	}
	return InputConsumed
}

// synthSliderHitAdapter is the legacy name kept so any external callers
// (or older tests) that reference the symbol continue to compile. It
// wraps the new knob adapter so behaviour is identical.
type synthSliderHitAdapter = synthControlHitAdapter

// synthSaveAsDialog hit adapters. The dialog itself is just a *Button
// pair + TextInput; these adapters wire the button presses into the
// shared OnClick closures the buttons already carry. Frame adapter
// consumes clicks inside the dialog rect so they don't fall through
// to the synth tab below.
type synthSaveAsDialogOKAdapter struct{ dv *DrumView }

func (h *synthSaveAsDialogOKAdapter) OnPress(x, y int) InputResult {
	if h.dv != nil {
		h.dv.ConfirmSaveAsDialog()
	}
	return InputCaptured
}
func (h *synthSaveAsDialogOKAdapter) OnDrag(x, y int)                     {}
func (h *synthSaveAsDialogOKAdapter) OnRelease(x, y int)                  {}
func (h *synthSaveAsDialogOKAdapter) OnWheel(x, y, steps int) InputResult { return InputIgnored }

type synthSaveAsDialogCancelAdapter struct{ dv *DrumView }

func (h *synthSaveAsDialogCancelAdapter) OnPress(x, y int) InputResult {
	if h.dv != nil {
		h.dv.CancelSaveAsDialog()
	}
	return InputCaptured
}
func (h *synthSaveAsDialogCancelAdapter) OnDrag(x, y int)                     {}
func (h *synthSaveAsDialogCancelAdapter) OnRelease(x, y int)                  {}
func (h *synthSaveAsDialogCancelAdapter) OnWheel(x, y, steps int) InputResult { return InputIgnored }

type synthSaveAsDialogFrameAdapter struct{ dv *DrumView }

func (h *synthSaveAsDialogFrameAdapter) OnPress(x, y int) InputResult { return InputConsumed }
func (h *synthSaveAsDialogFrameAdapter) OnDrag(x, y int)              {}
func (h *synthSaveAsDialogFrameAdapter) OnRelease(x, y int)           {}
func (h *synthSaveAsDialogFrameAdapter) OnWheel(x, y, steps int) InputResult {
	return InputIgnored
}

// ---- Test accessors ----------------------------------------------------

// SynthTabSliders exposes the slider value-store for tests.
func (p *synthPanel) SynthTabSliders() []*Slider {
	dv := p.dv
	return dv.instEditorSliders
}

// SynthTabControls exposes the visible Knob widgets for tests.
func (p *synthPanel) SynthTabControls() []*Knob {
	dv := p.dv
	return dv.instEditorControls
}

// SynthTabBindings exposes the widget→param map for tests.
func (p *synthPanel) SynthTabBindings() []instParamBinding {
	dv := p.dv
	return dv.instEditorBindings
}

// SynthTabButtons exposes the editor buttons (currently just reset).
func (p *synthPanel) SynthTabButtons() []*Button {
	dv := p.dv
	return dv.instEditorBtns
}

// SynthTabSections exposes the section card layout for tests.
func (p *synthPanel) SynthTabSections() []synthSection {
	dv := p.dv
	return dv.instEditorSections
}

// SelectSynthSectionByLabel opens the stage whose chip label matches
// (case-insensitive, e.g. "OSC", "ENVELOPE"). Routes through the same
// selection path as a chip tap (minus the ghost-enable side effect) so the
// JS bridge and scene catalog drive exactly what a user click drives.
// Returns false when no chip carries the label, and in the production UI
// (stages are developer tools, devtools.go).
func (p *synthPanel) SelectSynthSectionByLabel(label string) bool {
	dv := p.dv

	instID := dv.synthTabActiveInstrument()
	if instID == "" || !devToolsOn() {
		return false
	}
	for _, sec := range dv.instEditorSections {
		if strings.EqualFold(sectionLabel(sec.id), label) {
			dv.setSelectedSynthSection(instID, sec.id)
			if dv.eqPanelZone != nil {
				dv.eqPanelZone.Invalidate()
			}
			return true
		}
	}
	return false
}

// SynthTabNoSynth reports whether the synth tab is showing its
// "instrument does not use the synth" banner (active row plays a WAV
// sample with no synth recipe). Exposed for tests.
func (p *synthPanel) SynthTabNoSynth() bool {
	dv := p.dv
	return dv.instEditorNoSynth
}

// SynthTabBannerRect exposes the no-synth banner geometry for tests.
func (p *synthPanel) SynthTabBannerRect() image.Rectangle {
	dv := p.dv
	return dv.instEditorBannerRect
}

// SynthTabHeaderButtons exposes the header retrigger / vol buttons so
// shared-widget discipline tests can walk them.
func (p *synthPanel) SynthTabHeaderButtons() []*Button {
	dv := p.dv
	return dv.instEditorHeaderBtns
}

// SynthSaveAsDialog returns the active Save As name dialog, or nil when
// none is open.
func (p *synthPanel) SynthSaveAsDialog() *synthSaveAsDialog {
	dv := p.dv
	return dv.saveAsDialog
}

// synthSaveDirty reports whether the active instrument has unsaved param
// changes relative to its bound recipe. Drives the Save button's spec
// switch (Primary when dirty, Secondary when clean) and the dialog's
// suggested default name.
func (p *synthPanel) synthSaveDirty(instID instrument.ID) bool {
	dv := p.dv
	_ = dv

	if instID == "" {
		return false
	}
	return audio.InstrumentDocModified(instID)
}

// SynthTabHeader exposes the header layout state for tests.
func (p *synthPanel) SynthTabHeader() synthHeaderLayout {
	dv := p.dv
	return dv.instEditorHeader
}

// SectionID returns the section's enum id (test-only accessor).
func (s synthSection) SectionID() synthSectionID { return s.id }

// Label returns the human-readable section label (test-only accessor).
func (s synthSection) Label() string { return sectionLabel(s.id) }

// KnobCount returns the number of knobs assigned to the section
// (test-only accessor).
func (s synthSection) KnobCount() int { return len(s.knobIdxs) }

// Rect returns the section card's bounding rectangle (test-only).
func (s synthSection) Rect() image.Rectangle { return s.rect }

// EnableParam returns the modular per-stage bypass toggle param name for the
// section ("" for bespoke drum/FM sections). Test-only accessor.
func (s synthSection) EnableParam() string { return s.enableParam }

// KnobIdxs returns the schema indices of the knobs placed in this section
// (test-only accessor).
func (s synthSection) KnobIdxs() []int { return s.knobIdxs }

// EnsureNoTrailingNewline is a tiny helper kept around so a fmt+strings
// import set is not silently dropped when the file is edited above.
func ensureSynthImports() string { return strings.ToUpper("") }

func (dv *DrumView) toggleSynthStage(instID instrument.ID, paramName string) {
	dv.synp().toggleSynthStage(instID, paramName)
}

func (dv *DrumView) setSelectedSynthSection(instID instrument.ID, id synthSectionID) {
	dv.synp().setSelectedSynthSection(instID, id)
}

func (dv *DrumView) synthSelectedSection() *synthSection {
	return dv.synp().synthSelectedSection()
}

func (dv *DrumView) synthStageEssential(instID instrument.ID, enableParam string) bool {
	return dv.synp().synthStageEssential(instID, enableParam)
}

func (dv *DrumView) synthDetailEnablePillRect() image.Rectangle {
	return dv.synp().synthDetailEnablePillRect()
}

func (dv *DrumView) synthStageEnableState(instID instrument.ID, sec *synthSection) synthEnableState {
	return dv.synp().synthStageEnableState(instID, sec)
}

func (dv *DrumView) applySynthStageToggleByID(instID instrument.ID, id synthSectionID) {
	dv.synp().applySynthStageToggleByID(instID, id)
}

func (dv *DrumView) buildSynthTab(contentR image.Rectangle, instID instrument.ID) {
	dv.synp().buildSynthTab(contentR, instID)
}

func (dv *DrumView) layoutSynthHeader(contentR image.Rectangle, headerH int, instID instrument.ID, mobile bool) {
	dv.synp().layoutSynthHeader(contentR, headerH, instID, mobile)
}

func (dv *DrumView) buildSynthHeaderButtons(resolved instrument.ID) {
	dv.synp().buildSynthHeaderButtons(resolved)
}

func (dv *DrumView) sectionGrid(id synthSectionID) *ControlGrid {
	return dv.synp().sectionGrid(id)
}

func (dv *DrumView) SynthSectionGrid(id synthSectionID) *ControlGrid {
	return dv.synp().SynthSectionGrid(id)
}

func (dv *DrumView) drawSynthDetailPane(dst *ebiten.Image, instID instrument.ID) {
	dv.synp().drawSynthDetailPane(dst, instID)
}

func (dv *DrumView) synthTabUpdate() bool {
	return dv.synp().synthTabUpdate()
}

func (dv *DrumView) synthSectionTierSplit(knobIdxs []int) (essential, advanced []int) {
	return dv.synp().synthSectionTierSplit(knobIdxs)
}

func (dv *DrumView) drawSynthTab(dst *ebiten.Image, contentR image.Rectangle, instID instrument.ID) {
	dv.synp().drawSynthTab(dst, contentR, instID)
}

func (dv *DrumView) drawSynthHeader(dst *ebiten.Image) {
	dv.synp().drawSynthHeader(dst)
}

func (dv *DrumView) drawSynthFooter(dst *ebiten.Image) {
	dv.synp().drawSynthFooter(dst)
}

func (dv *DrumView) handleSynthTabInput(x, y int, pressed bool, instID instrument.ID) bool {
	return dv.synp().handleSynthTabInput(x, y, pressed, instID)
}

func (dv *DrumView) syncSliderFromKnob(idx int) {
	dv.synp().syncSliderFromKnob(idx)
}

func (dv *DrumView) propagateSynthSliderValue(idx int, instID instrument.ID) {
	dv.synp().propagateSynthSliderValue(idx, instID)
}

func (dv *DrumView) previewActiveSynth() {
	dv.synp().previewActiveSynth()
}

func (dv *DrumView) auditionInstrumentAfterDrag(instID instrument.ID) {
	dv.synp().auditionInstrumentAfterDrag(instID)
}

func (dv *DrumView) resolveSynthInstrument(instID instrument.ID) instrument.ID {
	return dv.synp().resolveSynthInstrument(instID)
}

func (dv *DrumView) synthTabActiveInstrument() instrument.ID {
	return dv.synp().synthTabActiveInstrument()
}

func (dv *DrumView) synthTabHitAreas() []HitArea {
	return dv.synp().synthTabHitAreas()
}

func (dv *DrumView) SynthTabSliders() []*Slider {
	return dv.synp().SynthTabSliders()
}

func (dv *DrumView) SynthTabControls() []*Knob {
	return dv.synp().SynthTabControls()
}

func (dv *DrumView) SynthTabBindings() []instParamBinding {
	return dv.synp().SynthTabBindings()
}

func (dv *DrumView) SynthTabButtons() []*Button {
	return dv.synp().SynthTabButtons()
}

func (dv *DrumView) SynthTabSections() []synthSection {
	return dv.synp().SynthTabSections()
}

func (dv *DrumView) SelectSynthSectionByLabel(label string) bool {
	return dv.synp().SelectSynthSectionByLabel(label)
}

func (dv *DrumView) SynthTabNoSynth() bool {
	return dv.synp().SynthTabNoSynth()
}

func (dv *DrumView) SynthTabBannerRect() image.Rectangle {
	return dv.synp().SynthTabBannerRect()
}

func (dv *DrumView) SynthTabHeaderButtons() []*Button {
	return dv.synp().SynthTabHeaderButtons()
}

func (dv *DrumView) SynthSaveAsDialog() *synthSaveAsDialog {
	return dv.synp().SynthSaveAsDialog()
}

func (dv *DrumView) synthSaveDirty(instID instrument.ID) bool {
	return dv.synp().synthSaveDirty(instID)
}

func (dv *DrumView) SynthTabHeader() synthHeaderLayout {
	return dv.synp().SynthTabHeader()
}

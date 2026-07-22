package ui

import (
	"fmt"
	"image"
	"image/color"
	"strconv"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
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
	// the generic post knobs + gain + post_enabled); it keeps its old name for
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
//	FILTER ENV · FORMANT · RESONATOR · POST
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
}

// synthSectionOrderForSchema returns the standardized section order for the
// given recipe schema, pruning empty sections. Every synth instrument is laid
// out with the same VOICE·OSC·FM·ENVELOPE·FILTER·POST sequence; only sections
// that have at least one knob or an enable pill survive. POST always survives
// (every recipe carries gain + post_enabled, or a generic post knob).
func synthSectionOrderForSchema(schema []audio.ParamDef) []synthSectionID {
	recipeID := schemaRecipeID(schema)
	kickActive := schemaUsesKickStage(schema)
	// Tally per-section content: a knob (grid param) or an enable pill (a
	// per-stage *_enabled toggle that is actually appended to this recipe).
	hasContent := map[synthSectionID]bool{}
	for _, d := range schema {
		if d.Group == audio.SynthHiddenGroup || synthKickParamHidden(kickActive, d) {
			continue
		}
		// Any non-hidden param present in the schema — a knob OR an enable
		// toggle — makes its standardized section non-empty. Collision-excluded
		// stage params are simply absent from the schema (the recipe never
		// appended them), so they can't conjure an empty stage card here.
		hasContent[sectionForParamDefIn(recipeID, d)] = true
	}
	out := make([]synthSectionID, 0, len(unifiedSynthSectionOrder))
	for _, sec := range unifiedSynthSectionOrder {
		if hasContent[sec] {
			out = append(out, sec)
		}
	}
	return out
}

// schemaRecipeID best-effort recovers which registered recipe a schema belongs
// to so the section router can tell an appended stage param (route by stage
// group) from a same-named family/generic knob (route by family rules). Matches
// by identical param-name sequence against the recipe registry. Returns "" when
// no registered recipe matches (e.g. a synthesized test schema), in which case
// the name-based router falls back to treating stage-named params as stages.
func schemaRecipeID(schema []audio.ParamDef) string {
	for id, reg := range audio.RecipeRegistrations() {
		if reg == nil || len(reg.Params) != len(schema) {
			continue
		}
		match := true
		for i := range schema {
			if reg.Params[i].Name != schema[i].Name {
				match = false
				break
			}
		}
		if match {
			return id
		}
	}
	return ""
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
// env_enabled, filter_enabled, drive_enabled, post_enabled) to its stage
// section. The POST section hosts both drive_enabled (the modular voice's post
// toggle) and post_enabled (the migrated recipes' post toggle) — only one is
// ever present on a given recipe, so there is no conflict.
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
	case "drive_enabled", "post_enabled":
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
	// Phase-8C modulator stages (Groups carried verbatim from
	// ModularSynthParamDefs). PITCH ENV reuses the PITCH section ID.
	case "pitchenv":
		return synthSectionPitch, true
	case "lfo":
		return synthSectionLFO, true
	case "burst":
		return synthSectionBurst, true
	// Configurable kick voice knobs (slot-1 kick generator, group "kick") are
	// family/voice knobs → the agnostic VOICE stage (no instrument-specific stage).
	case "kick":
		return synthSectionVoice, true
	}
	return 0, false
}

// schemaUsesKickStage reports whether a recipe's param SCHEMA drives the
// configurable KICK stage — its kick voice is active (gen-slot-1 source == 5).
// The kick params live in the SHARED modular schema and every modular recipe has
// an identical param-NAME sequence (so schemaRecipeID can't tell them apart);
// the per-recipe signal is the SEEDED default of gen1_source (5 for a modular
// kick instrument like dnb-kick, 0/1/2 for everything else). Without this gate
// every modular instrument (violin, cello, organ, …) would show an inert KICK
// card.
func schemaUsesKickStage(schema []audio.ParamDef) bool {
	for _, d := range schema {
		if d.Name == "gen1_source" {
			return d.Default == 5
		}
	}
	return false
}

// synthKickParamHidden reports whether a "kick"-group param (kick_enabled or a
// surfaced slot-1 kick knob) must be hidden — true for every non-kick recipe
// (kickActive precomputed once per build via schemaUsesKickStage), so the KICK
// card appears only on kick instruments.
func synthKickParamHidden(kickActive bool, def audio.ParamDef) bool {
	// The kick generator IS the voice, so it's always on — hide its enable toggle
	// on every recipe (VOICE carries no enable pill, matching snare/cym/bass).
	if def.Name == "kick_enabled" {
		return true
	}
	// The kick voice knobs are family knobs in VOICE; hide them on non-kick
	// recipes, exactly as a family's knobs only appear on that family's recipe.
	return def.Group == "kick" && !kickActive
}

// synthOscModelParamHidden hides the physical-model OSC params (osc_sax_* /
// osc_bow_*) unless the matching oscillator is actually selected — so a beginner
// only sees "Bow Pressure" when the Bowed String oscillator is chosen, and the
// sax reed knobs only under the Sax oscillator. oscType is the instrument's
// current osc_type (seed default + user edits). Sax = 11, Bowed String = 7.
func synthOscModelParamHidden(def audio.ParamDef, oscType float64) bool {
	ot := int(oscType + 0.5)
	if strings.HasPrefix(def.Name, "osc_sax_") {
		return ot != 11
	}
	if strings.HasPrefix(def.Name, "osc_bow_") {
		return ot != 7
	}
	return false
}

// sectionForParamDefIn is the canonical Phase-8B router: given the recipe a
// param belongs to, it routes the param into one of the standardized sections.
//
//   - APPENDED stage params (osc_*/fm_*/amp_*/filter_*/gain/post_enabled and
//     the *_enabled toggles, when actually appended to recipeID) route by their
//     canonical stage Group → OSC/FM/ENVELOPE/FILTER/POST.
//   - Generic post knobs (pitch/decay/tone/drive/body/brightness) → POST, EXCEPT
//     decay which is envelope-shaped → ENVELOPE.
//   - Everything else (the per-instrument family knobs — kick_*, snare_*, cym_*,
//     bass_*, the fm-* recipes' fm_* voice operators, the <family>_wave generator
//     picker, and fundamental) → VOICE. These knobs ARE the instrument's sound.
//
// recipeID == "" (synthesized/test schema with no registry match) treats any
// stage-named param as an appended stage so a hand-built modular-shaped schema
// still routes by group.
func sectionForParamDefIn(recipeID string, def audio.ParamDef) synthSectionID {
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
	// Configurable kick voice knobs (group "kick", the surfaced slot-1 gen-bank
	// kick generator). These are the kick's FAMILY/voice knobs, so they live in
	// the agnostic VOICE stage — exactly like snare_*/cym_*/bass_* family knobs —
	// rather than an instrument-specific KICK stage. The visibility gate
	// (synthKickParamHidden) keeps them off non-kick recipes, just as VOICE only
	// shows a given family's knobs on that family's recipe.
	if def.Group == "kick" {
		return synthSectionVoice
	}
	appended := recipeID == "" || audio.IsAppendedStageName(recipeID, name)
	if appended {
		if sec, ok := modularStageGroupSection(def.Group); ok {
			return sec
		}
	}
	// Generic post knobs → POST (decay is envelope-shaped → ENVELOPE).
	switch name {
	case "decay":
		return synthSectionEnvelope
	case "pitch", "drive", "tone", "body", "brightness":
		return synthSectionPost
	}
	// Family/voice knobs (including fm-* recipes' fm_* operators, the
	// <family>_wave generator picker, and fundamental) → VOICE.
	return synthSectionVoice
}

// sectionForParam is the recipe-agnostic router used by tests and the legacy
// (no-recipe-context) callers. It treats any stage-named param as an appended
// stage param (route by stage group), matching sectionForParamDefIn("").
func sectionForParam(name string) synthSectionID {
	if def, ok := audio.StageParamDefByName(name); ok {
		return sectionForParamDefIn("", def)
	}
	return sectionForParamDefIn("", audio.ParamDef{Name: name})
}

// sectionForParamDef routes a ParamDef without recipe context. Stage params
// route by their canonical Group; family/generic knobs by the name rules.
func sectionForParamDef(def audio.ParamDef) synthSectionID {
	return sectionForParamDefIn("", def)
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
//   - synthKickParamHidden: the shared kick-voice knobs on a non-kick recipe.
//   - synthOscModelParamHidden: the physical-model OSC knobs (osc_sax_*/osc_bow_*)
//     unless their oscillator is the one selected. These are declared on EVERY
//     modular recipe but are waveguide-internal to one oscillator each, so on a
//     piano/kick/organ they are inert and correctly have no knob.
func synthParamHiddenInContext(def audio.ParamDef, kickActive bool, oscType float64) bool {
	return def.Group == audio.SynthHiddenGroup ||
		synthKickParamHidden(kickActive, def) ||
		synthOscModelParamHidden(def, oscType)
}

// synthParamIsGridControlIn reports whether a ParamDef is SELECTABLE as a knob in
// the section grid of a recipe with the given context (kick stage active?
// current oscillator?). This is the contextual form of synthParamIsGridControl and
// the one any per-recipe assertion must use: a param that is hidden in context
// has no knob to select and therefore no focus graph to be right or wrong about.
func synthParamIsGridControlIn(def audio.ParamDef, kickActive bool, oscType float64) bool {
	return !synthParamHiddenInContext(def, kickActive, oscType) && !isSynthEnableParam(def.Name)
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
func synthStageEnabled(instID, paramName string) bool {
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
func (p *synthPanel) synthStageEssential(instID, enableParam string) bool {
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
func (p *synthPanel) toggleSynthStage(instID, paramName string) {
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
	dv.requestSynthMirror(instID) // toggling a stage changes the sound
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
		id == synthSectionResonator
}

// synthStageEnableState resolves the enable pill for sec on instID.
func (p *synthPanel) synthStageEnableState(instID string, sec *synthSection) synthEnableState {
	if sec == nil {
		return synthEnableState{}
	}
	switch {
	case sec.id == synthSectionVoice:
		// The voice is the instrument — mandatory, always on, never disable-able.
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
// value — the recipe's shipped default when that is itself active, otherwise
// onFallback.
func (p *synthPanel) synthGateEnableState(instID, param string, threshold, offVal, onFallback float64) synthEnableState {
	resolved := p.dv.resolveSynthInstrument(instID)
	cur := audio.MergedInstrumentParamsRO(resolved)[param]
	onVal := onFallback
	if sd, ok := audio.RecipeShippedDefaults(audio.RecipeForInstrument(resolved))[param]; ok && sd >= threshold {
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
func (p *synthPanel) applySynthStageToggle(instID string, sec *synthSection) {
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
	dv.requestSynthMirror(resolved)
	dv.commitInstrumentParams(resolved)
}

// applySynthStageToggleByID resolves the live section for id and toggles it.
// The tree hit-adapter carries the id (sections are rebuilt each layout, so a
// pointer would dangle).
func (p *synthPanel) applySynthStageToggleByID(instID string, id synthSectionID) {
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

// synthChip is one collapsed stage in the pipeline chip strip (synth-tab
// redesign). Chips render in audio-pipeline order, connected by a wire;
// exactly one chip is selected — its stage expands into the detail pane.
// Disabled stages stay visible as dimmed ghost chips so the full signal
// order never disappears.
type synthChip struct {
	id          synthSectionID
	rect        image.Rectangle
	enableParam string
	enabled     bool
	selected    bool
}

// selectedSynthSection resolves which stage is open for instID. A stored
// per-session selection wins when its section still exists for the active
// recipe; otherwise the default is the first section in pipeline order that
// has knobs AND whose stage is enabled (→ VOICE for drums, OSC for modular),
// falling back to the first knobbed section, then the first section. The
// default is a pure function of the section model (never written back), so
// layout stays deterministic for fingerprint tests.
func (p *synthPanel) selectedSynthSection(instID string, sections []synthSection) synthSectionID {
	dv := p.dv

	if id, ok := dv.instEditorSelectedSection[instID]; ok {
		for i := range sections {
			if sections[i].id == id {
				return id
			}
		}
	}
	fallback := synthSectionID(-1)
	for i := range sections {
		s := &sections[i]
		if len(s.knobIdxs) == 0 {
			continue
		}
		if fallback == -1 {
			fallback = s.id
		}
		if s.enableParam == "" || synthStageEnabled(instID, s.enableParam) {
			return s.id
		}
	}
	if fallback != -1 {
		return fallback
	}
	if len(sections) > 0 {
		return sections[0].id
	}
	return -1
}

// setSelectedSynthSection records the open stage for instID. Lazily
// allocates the map (once per DrumView, never per frame). The key is the
// RESOLVED instrument id — buildSynthTab reads the map with the resolved id,
// while hit-area adapters carry synthTabActiveInstrument()'s raw value, so
// normalize here to keep every caller consistent.
func (p *synthPanel) setSelectedSynthSection(instID string, id synthSectionID) {
	dv := p.dv

	instID = dv.resolveSynthInstrument(instID)
	if instID == "" {
		return
	}
	if dv.instEditorSelectedSection == nil {
		dv.instEditorSelectedSection = map[string]synthSectionID{}
	}
	dv.instEditorSelectedSection[instID] = id
}

// synthSelectedSection returns the section currently expanded in the detail
// pane (the one whose chip is selected), or nil when the synth tab has no
// sections laid out.
func (p *synthPanel) synthSelectedSection() *synthSection {
	dv := p.dv

	for _, c := range dv.instEditorChips {
		if !c.selected {
			continue
		}
		for i := range dv.instEditorSections {
			if dv.instEditorSections[i].id == c.id {
				return &dv.instEditorSections[i]
			}
		}
	}
	return nil
}

// synthDetailEnablePillRect returns the rect of the enable toggle pill in the
// detail-pane header for the selected stage. Empty when the selected stage
// carries no pill (synthSectionHasPill) or the pane is too small. Every shown
// stage has a pill now — boolean (*_enabled), value-gated (ENSEMBLE/RESONATOR),
// or the mandatory locked VOICE. Replaces the per-card sectionEnablePillRect
// placement that used to collide with titles and knobs on narrow cards.
func (p *synthPanel) synthDetailEnablePillRect() image.Rectangle {
	dv := p.dv

	sel := dv.synthSelectedSection()
	detail := dv.instEditorDetailR
	if sel == nil || !synthSectionHasPill(sel.id, sel.enableParam) || detail.Empty() {
		return image.Rectangle{}
	}
	pillW := FXToggleTrackW() + 2*SpaceXS
	pillH := FXToggleTrackH() + 2*SpaceXS
	headerH := dv.instEditorDetailHeaderH
	if pillH > headerH+SpaceXS {
		// Degenerate-short pane (test-harness sizes): no room for the pill
		// without overlapping the knob area — the exact collision this
		// redesign removes. Production panels keep the full header band.
		return image.Rectangle{}
	}
	x1 := detail.Max.X - synthSectionPaddingX
	x0 := x1 - pillW
	y0 := detail.Min.Y + (headerH-pillH)/2
	if y0 < detail.Min.Y+2 {
		y0 = detail.Min.Y + 2
	}
	y1 := y0 + pillH
	if x0 <= detail.Min.X+synthSectionPaddingX || y1 >= detail.Max.Y {
		return image.Rectangle{}
	}
	return image.Rect(x0, y0, x1, y1)
}

// synthHeaderLayout records the rects + cached fingerprint for the
// header strip — instrument label + recipe id, waveform thumbnail, and
// the three action buttons (Save, Save As, Reset). With the buttons
// living in the header, sections own the full vertical space below.
type synthHeaderLayout struct {
	rect         image.Rectangle
	waveformRect image.Rectangle
	captionRect  image.Rectangle
	resetRect    image.Rectangle
	saveRect     image.Rectangle
	saveAsRect   image.Rectangle
	// previewRect hosts the Preview button (one-shot audition of the
	// current unsaved knob state via synthAuditionFn — same plumbing as
	// the knob-release audition, mirroring the Sampler tab's Preview).
	previewRect image.Rectangle
	instLabel   string
	recipeID    string
}

// ---- Legacy / compat types ---------------------------------------------

// instParamBinding maps a widget index to a single ParamDef. Lives on
// DrumView state (one set of bindings per row's active recipe).
type instParamBinding struct {
	def audio.ParamDef
}

// synthResetButtonTag / synthSaveButtonTag / synthSaveAsButtonTag are
// the sentinel button texts that identify the footer buttons in the
// instEditorBtns slice. Stored in Button.Text because we already index
// the slice positionally and route via the text in synthTabHitAreas
// (Phase 4 added Save + Save-As alongside the existing Reset). The
// leading \x00 keeps them out of any text-equality path that might
// surface them on screen.
const (
	synthResetButtonTag   = "\x00synth-reset"
	synthSaveButtonTag    = "\x00synth-save"
	synthSaveAsButtonTag  = "\x00synth-save-as"
	synthPreviewButtonTag = "\x00synth-preview"
)

// ---- Layout constants --------------------------------------------------

const (
	// Narrow header — just tall enough for the action buttons + a
	// little breathing room around the caption. Most of the panel
	// belongs to the knobs.
	// Header / section chrome — fixed because they encode the visual
	// rhythm of the panel rather than the interactive control sizing.
	// (Phase 2 audio-panel design-system compliance moved every
	// density-tied dimension to DESIGN.md `densities:` block; these
	// padding/title constants stay so the chrome geometry is
	// pre-density-deterministic and tests don't need to re-snapshot
	// per density tier.)
	synthSectionPaddingX    = 8
	synthSectionPaddingY    = 6
	synthSectionTitleHeight = 16
	// Header strip height is the profileOverride densities token SynthHeaderH
	// (desktop 48 / mobile 38), read via Profile().SynthHeaderH so the synth
	// tab header is re-styleable from DESIGN.md per screen class.
)

// instrumentHasSynthControls reports whether an instrument id resolves to a
// synth recipe with at least one editable param. It is the single classifier
// for "is this a synth, or a WAV sample?" and MUST stay in lockstep with the
// no-synth detection in buildSynthTab (recipeID == "" || len(schema) == 0 ⇒
// WAV sample, banner shown). The Synth tab selector is greyed/disabled when
// this returns false for the active instrument.
func instrumentHasSynthControls(instID string) bool {
	if instID == "" {
		return false
	}
	recipeID := audio.RecipeForInstrument(instID)
	if recipeID == "" {
		return false
	}
	reg, ok := audio.RecipeRegistrations()[recipeID]
	return ok && reg != nil && len(reg.Params) > 0
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
func (p *synthPanel) buildSynthTab(contentR image.Rectangle, instID string) {
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
	// Stage 4: ensure the right-pane mirror has content as soon as the Synth tab
	// is built/activated. Coalesced by params hash, so re-building the same tab
	// each Layout is a no-op after the first render.
	dv.requestSynthMirror(resolved)

	recipeID := audio.RecipeForInstrument(resolved)
	var schema []audio.ParamDef
	if recipeID != "" {
		if reg, ok := audio.RecipeRegistrations()[recipeID]; ok && reg != nil {
			schema = reg.Params
		}
	}

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
	// so a fresh MergeRecipeDefaults here was a per-frame ~115 KB allocation.
	genMerged := audio.MergedInstrumentParamsRO(resolved)
	deferred := false
	// Defer any schema CHANGE while the value-wheel popup is open. The popup
	// caches the knob pointer and edits it BY INDEX (see
	// openSynthControlWheelPopup): a schema swap mid-edit rebuilds the binding
	// slice, so the popup's index would bind a DIFFERENT param and the
	// in-flight edit would scramble it (in WASM that silently drove foreign
	// params like gain/osc_enabled, silencing the voice until reload). Hold the
	// current knob set until the popup closes; the next Layout swaps cleanly.
	popupEditing := dv.synthWheelPopup != nil && dv.synthWheelPopup.IsOpen()
	if popupEditing && !bindingsMatchSchema(dv.instEditorBindings, schema) {
		schema = schemaFromBindings(dv.instEditorBindings)
		deferred = true
	}
	if synthDragDebug {
		dv.sdbgBuildTab(fmt.Sprintf("inst=%s deferred=%v popupEditing=%v prevBindings=%d schemaLen=%d",
			resolved, deferred, popupEditing, len(dv.instEditorBindings), len(schema)))
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
			if headerH < 0 {
				headerH = 0
			}
		}
	}

	// Header strip occupies the top band; sections fill everything
	// below it down to contentR.Max.Y. Footer-button rects are placed
	// inside the header by layoutSynthHeader.
	dv.layoutSynthHeader(contentR, headerH, resolved, recipeID, mobile)

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
	} else if mobile {
		dv.instEditorPreviewRect = image.Rectangle{}
		// Mobile: the "Your sound" mirror + focus graph live in a RIGHT column
		// beside the knob grid, set in layoutSynthSections once detailR is known
		// (NOT a top band). Cleared here; the sections area keeps its full height
		// so the chip strip + knob grid get the room.
		dv.instEditorMobileFocusRect = image.Rectangle{}
	} else {
		// Narrow desktop panel with no preview pane: no mobile band either.
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
		dv.sdbg("[synthdrag] buildSynthTab REBUILDING knob slice (reuseExisting=false) — live edit state will be LOST; popupEditing=%v", dv.synthWheelPopup != nil && dv.synthWheelPopup.IsOpen())
	}
	if !reuseExisting {
		dv.instEditorControls = dv.instEditorControls[:0]
		dv.instEditorSliders = dv.instEditorSliders[:0]
		dv.instEditorStepBadges = dv.instEditorStepBadges[:0]
		dv.instEditorBindings = dv.instEditorBindings[:0]
	}
	kickActive := schemaUsesKickStage(schema)
	oscType := genMerged["osc_type"] // current oscillator (seed + user); gates the physical-model knobs
	for i, def := range schema {
		sec := sectionForParamDefIn(recipeID, def)
		switch {
		case synthParamHiddenInContext(def, kickActive, oscType):
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
			// dispatch uses via recipeParamsToModular).
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

	dv.instEditorSectionsRect = sectionsRect
	dv.layoutSynthSections(sectionsRect, sections, mobile, resolved)
	dv.instEditorSections = sections

	// The Save / SaveAs / Reset buttons live inside the header strip
	// (right of the caption). buildSynthHeaderButtons populates
	// dv.instEditorBtns using the rects layoutSynthHeader reserved.
	dv.buildSynthHeaderButtons(resolved)

	// Section-card layout doesn't scroll vertically. Reset scroll fields
	// so the legacy fingerprint test (TestSynthLayoutFingerprintStable)
	// stays deterministic.
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

// layoutSynthHeader fills dv.instEditorHeader. The header strip carries
// the waveform thumbnail, the instrument/recipe caption, and the three
// action buttons (Save, Save As, Reset) right-aligned alongside the
// caption. The footer band is gone — sections own the full vertical
// space below the header, giving knobs back the breathing room they
// had before the Save buttons were added.
func (p *synthPanel) layoutSynthHeader(contentR image.Rectangle, headerH int, instID, recipeID string, mobile bool) {
	dv := p.dv

	if mobile {
		// Mobile relocates the header actions to the sticky bar and drops the
		// identity banner — nothing is laid out in-panel. An empty header makes
		// buildSynthHeaderButtons skip every button and drawSynthHeader
		// early-return (rect empty).
		dv.instEditorHeader = synthHeaderLayout{}
		dv.instEditorHeaderBtns = dv.instEditorHeaderBtns[:0]
		return
	}

	r := image.Rect(contentR.Min.X, contentR.Min.Y, contentR.Max.X, contentR.Min.Y+headerH)
	dv2 := Profile().DensityValues()
	inset := SpaceMD
	innerY0 := r.Min.Y + SpaceSM
	innerY1 := r.Max.Y - SpaceSM
	// Thumbnail width is a density token (clamped to a third of the header
	// so a narrow panel doesn't let the thumbnail crowd out the caption).
	thumbW := dv2.SynthHeaderThumbW
	if thumbW > r.Dx()/3 {
		thumbW = r.Dx() / 3
	}
	waveformR := image.Rect(r.Min.X+inset, innerY0, r.Min.X+inset+thumbW, innerY1)

	// Action buttons (Preview, Save, Save As, Reset). The layout is FIXED
	// at every width — all four actions are always laid out as visible
	// buttons, never collapsed behind an overflow chevron. Desktop keeps
	// them inline on a single row right of the caption; mobile drops them
	// onto a dedicated full-width action row below the caption so even the
	// widest localized labels stay untruncated.
	btnH := dv2.SynthHeaderButtonH
	if innerH := innerY1 - innerY0; innerH > 0 && btnH > innerH && !mobile {
		btnH = innerH
	}
	rightEdge := r.Max.X - inset

	var previewR, saveR, saveAsR, resetR, captionR image.Rectangle

	if mobile {
		// Two-row header: caption (truncated) on top, then a full-width row
		// of four EQUAL-width action cells. Spanning the full content width
		// gives every cell a quarter of the panel, which is wide enough for
		// the longest localized label (es-419 "Guardar como") at the
		// narrowest supported phone width — so all four stay untruncated and
		// the layout is identical at every width (verified by
		// TestSynthHeaderActionsAlwaysVisibleNoOverflow for EN + es-419).
		captionBottom := innerY1 - btnH - SpaceSM
		if captionBottom < innerY0+TextHeight() {
			captionBottom = innerY0 + TextHeight()
		}
		captionR = image.Rect(waveformR.Max.X+SpaceSM, innerY0, rightEdge, captionBottom)

		rowTop := innerY1 - btnH
		rowLeft := r.Min.X + inset
		cellW := (rightEdge - rowLeft - 3*SpaceSM) / 4
		if cellW < 1 {
			cellW = 1
		}
		x := rowLeft
		previewR = image.Rect(x, rowTop, x+cellW, rowTop+btnH)
		x = previewR.Max.X + SpaceSM
		saveR = image.Rect(x, rowTop, x+cellW, rowTop+btnH)
		x = saveR.Max.X + SpaceSM
		saveAsR = image.Rect(x, rowTop, x+cellW, rowTop+btnH)
		x = saveAsR.Max.X + SpaceSM
		// Reset claims the remainder so rounding slack never leaves a gap.
		resetR = image.Rect(x, rowTop, rightEdge, rowTop+btnH)
	} else {
		previewW := labelButtonWidth(synthActionLabel("preview"))
		saveW := labelButtonWidth(synthActionLabel("save"))
		saveAsW := labelButtonWidth(synthActionLabel("save-as"))
		resetW := labelButtonWidth(synthActionLabel("reset"))
		btnY0 := (r.Min.Y+r.Max.Y)/2 - btnH/2
		btnY1 := btnY0 + btnH

		captionLeft := waveformR.Max.X + SpaceSM
		totalBtnW := previewW + SpaceSM + saveW + SpaceSM + saveAsW + SpaceSM + resetW
		// Caption gets whatever fits between the waveform and the button
		// group, with a comfortable upper bound on wide panels so the
		// buttons stay grouped near the top-left rather than drifting to
		// the right edge.
		captionMaxW := 240
		captionRight := rightEdge - totalBtnW - SpaceMD
		if captionRight > captionLeft+captionMaxW {
			captionRight = captionLeft + captionMaxW
		}
		if captionRight < captionLeft+40 {
			captionRight = captionLeft + 40
		}
		captionR = image.Rect(captionLeft, innerY0, captionRight, innerY1)

		// Place buttons a SpaceMD gap right of the caption. Preview leads the
		// group (mirrors the Sampler tab's preview-then-save flow).
		btnX := captionR.Max.X + SpaceMD
		previewR = image.Rect(btnX, btnY0, btnX+previewW, btnY1)
		saveR = image.Rect(previewR.Max.X+SpaceSM, btnY0, previewR.Max.X+SpaceSM+saveW, btnY1)
		saveAsR = image.Rect(saveR.Max.X+SpaceSM, btnY0, saveR.Max.X+SpaceSM+saveAsW, btnY1)
		resetR = image.Rect(saveAsR.Max.X+SpaceSM, btnY0, saveAsR.Max.X+SpaceSM+resetW, btnY1)
	}

	dv.instEditorHeader = synthHeaderLayout{
		rect:         r,
		waveformRect: waveformR,
		captionRect:  captionR,
		resetRect:    resetR,
		saveRect:     saveR,
		saveAsRect:   saveAsR,
		previewRect:  previewR,
		instLabel:    instID,
		recipeID:     recipeID,
	}
	dv.instEditorHeaderBtns = dv.instEditorHeaderBtns[:0]
}

// ensureMobileActionButtons builds the persistent [Preview][Save][⋯] cluster
// hosted on the sticky bar on mobile (Approach B). Built once; the buttons keep
// their OnClick behavior for the panel's lifetime while the sticky bar owns
// their per-frame layout.
func (p *synthPanel) ensureMobileActionButtons() {
	dv := p.dv
	if p.mobilePreviewBtn == nil {
		p.mobilePreviewBtn = NewSpecButton(synthActionLabel("preview"), ComponentButtonSecondary, func() {
			dv.previewActiveSynth()
		})
	}
	if p.mobileSaveBtn == nil {
		p.mobileSaveBtn = NewSpecButton(synthActionLabel("save"), ComponentButtonSecondary, func() {
			dv.SaveActiveRecipe()
		})
	}
	if p.mobileOverflowBtn == nil {
		p.mobileOverflowBtn = NewSpecButton("", ComponentButtonSecondary, func() {
			dv.openSynthActionMenu(p.mobileOverflowBtn.Rect())
		})
		p.mobileOverflowBtn.Icon = string(IconOverflow)
	}
}

// MobileActionButtons returns the Synth tab's trailing action cluster for the
// sticky bar ([Preview][Save][⋯]), or nil off-mobile / when the active
// instrument has no synth controls (a loaded-WAV instrument has nothing to
// save). Refreshes localized labels and the Save dirty-spec each call.
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
	p.mobileSaveBtn.Text = synthActionLabel("save")
	saveSpec := ComponentButtonSecondary
	if dv.synthSaveDirty(resolved) {
		saveSpec = ComponentButtonPrimary
	}
	p.mobileSaveBtn.SetSpec(saveSpec)
	return []*Button{p.mobilePreviewBtn, p.mobileSaveBtn, p.mobileOverflowBtn}
}

// buildSynthHeaderButtons constructs the three *Button widgets that sit
// inside the header strip. Save's spec toggles Primary↔Secondary by
// dirty state so the visual signal stays honest. Empty rects (very
// narrow panels) just skip the corresponding button entirely.
func (p *synthPanel) buildSynthHeaderButtons(resolved string) {
	dv := p.dv

	h := dv.instEditorHeader
	if !h.previewRect.Empty() {
		previewBtn := NewSpecButton(synthPreviewButtonTag, ComponentButtonSecondary, func() {
			dv.previewActiveSynth()
		})
		previewBtn.SetRect(h.previewRect)
		dv.instEditorBtns = append(dv.instEditorBtns, previewBtn)
	}
	if !h.saveRect.Empty() {
		saveSpec := ComponentButtonSecondary
		if dv.synthSaveDirty(resolved) {
			saveSpec = ComponentButtonPrimary
		}
		saveBtn := NewSpecButton(synthSaveButtonTag, saveSpec, func() {
			dv.SaveActiveRecipe()
		})
		saveBtn.SetRect(h.saveRect)
		dv.instEditorBtns = append(dv.instEditorBtns, saveBtn)
	}
	if !h.saveAsRect.Empty() {
		saveAsBtn := NewSpecButton(synthSaveAsButtonTag, ComponentButtonSecondary, func() {
			dv.openSaveAsDialog()
		})
		saveAsBtn.SetRect(h.saveAsRect)
		dv.instEditorBtns = append(dv.instEditorBtns, saveAsBtn)
	}
	if !h.resetRect.Empty() {
		resetBtn := NewSpecButton(synthResetButtonTag, ComponentButtonSecondary, func() {
			dv.ResetActiveRecipe()
		})
		resetBtn.SetRect(h.resetRect)
		dv.instEditorBtns = append(dv.instEditorBtns, resetBtn)
	}
}

// layoutSynthSections computes the geometry for the 4 section cards.
// Desktop lays them side-by-side; mobile stacks each section as a
// full-width row. Per-instrument Delay/Reverb sends live in the per-row
// FX overlay (opened from the row rack), so the Synth tab no longer
// surfaces an OUT column.
// layoutSynthSections lays out the pipeline chip strip + expand-one detail
// pane (synth-tab redesign). Collapsed stages become compact chips in audio
// order at the top of the area; the selected stage's knobs fill the detail
// pane below at full size. Collapsed sections get an empty rect, so
// placeKnobsInSection gives all their knobs empty rects — they are neither
// drawn nor hit-tested, while the knob INSTANCES (and binding indices)
// survive untouched, preserving in-flight drag state exactly like the
// scrolled-off-window path always has.
func (p *synthPanel) layoutSynthSections(rowR image.Rectangle, sections []synthSection, mobile bool, instID string) {
	dv := p.dv

	dv.instEditorChips = dv.instEditorChips[:0]
	dv.instEditorDetailR = image.Rectangle{}
	inset := SpaceMD
	rowR.Min.X += inset
	rowR.Max.X -= inset
	rowR.Min.Y += SpaceSM
	rowR.Max.Y -= SpaceSM
	if rowR.Empty() || len(sections) == 0 {
		for i := range sections {
			sections[i].rect = image.Rectangle{}
			dv.placeKnobsInSection(sections[i], mobile)
		}
		return
	}

	selID := dv.selectedSynthSection(instID, sections)
	dv2 := Profile().DensityValues()
	chipH := dv2.SynthChipH
	// Per-row strip band height. SynthChipStripH is intentionally taller than
	// SynthChipH so the pipeline reads as a band with vertical breathing room
	// rather than chips flush against the header/detail edges; the chip is
	// centred within each band.
	stripRowH := dv2.SynthChipStripH
	if stripRowH < chipH {
		stripRowH = chipH
	}
	// Degenerate-fit: on very short panels (the 640×480 test harness gives
	// the whole audio panel 80 px) the strip cedes height to the detail
	// pane — knobs keep priority, exactly like the old card layout's
	// graceful shrink. Clamp the BAND (and the chip within it) so the strip
	// never claims more than a third of the panel. Production desktop panels
	// (~240 px+) never hit this.
	if maxStrip := (rowR.Dy() - SpaceSM) / 3; stripRowH > maxStrip {
		stripRowH = maxStrip
		if stripRowH < 12 {
			stripRowH = 12
		}
		if chipH > stripRowH {
			chipH = stripRowH
		}
	}
	chipMinW := dv2.SynthChipMinW
	chipInset := (stripRowH - chipH) / 2
	gap := SpaceSM // hosts the connector wire between adjacent chips
	rowGap := SpaceXS

	n := len(sections)
	x := rowR.Min.X
	y := rowR.Min.Y
	if mobile {
		// Mobile: greedy wrap. Every stage stays visible at 360 px —
		// wrapping rows beat a horizontal scroll strip, which would hide
		// pipeline stages off-screen and fight the detail pane's vertical
		// scroll for gestures.
		//
		// Detail-pane floor: with the always-on focus band stealing the top of
		// the sections area, a deep (multi-row) wrapped strip could otherwise
		// consume the entire remaining grid and leave the detail/knob pane empty.
		// Pre-count the wrap rows for the current chip metrics; if the strip
		// would overflow `rowR.Dy() - minDetailH`, COMPACT the strip rows (and
		// the chip within them) so every stage still wraps visibly while the
		// detail pane keeps a usable floor. The knob grid inside detail scrolls.
		// Detail-pane reserve: a deep (multi-row) wrapped strip must not starve the
		// knob pane. The strip COMPACTS (smaller per-row band) so the detail pane
		// keeps a usable height; the knob grid inside detail scrolls.
		//
		// Both bounds derive from the live density so they can't drift from the
		// knob-row size the "knobs aren't cramped" guard
		// (TestMobileSynthTab_ControlColumnHasUsableHeight) asserts:
		//   - minDetailH (hard floor): title row + ONE knob row — survives the
		//     headerH shrink clamp at synthSectionTitleHeight below. A fixed 72 left
		//     only ~56 px of body once the 10-stage pipeline (FILTER ENV) compacted
		//     the strip onto the floor.
		//   - twoRowDetail (preferred): title row + TWO knob rows, reserved whenever
		//     the panel is tall enough that the wrapped strip can still compact in
		//     alongside it (the taller 390×896 viewport). Capping the strip to leave
		//     this much hands the surplus to the knob pane instead of letting the
		//     strip greedily claim full-height rows.
		knobRow := dv2.SynthControlMin + dv2.SynthControlCaptionH
		minDetailH := synthSectionTitleHeight + knobRow + 2*synthSectionPaddingY
		// Two-row reserve sits BELOW the FULL detail header (it only applies when the
		// pane is tall, so headerH does not shrink), hence SynthDetailHeaderH — not
		// the bare title row — plus two knob rows of body.
		twoRowDetail := dv2.SynthDetailHeaderH + 2*knobRow + 2*synthSectionPaddingY
		if rows := synthChipWrapRows(sections, rowR, chipMinW, gap); rows >= 1 {
			// Prefer the two-row reserve, but only if the strip can still fit `rows`
			// at its minimum 12-px band alongside it; otherwise fall back to the
			// one-row floor.
			detailReserve := minDetailH
			if rowR.Dy()-twoRowDetail-SpaceSM >= rows*12+rowGap*(rows-1) {
				detailReserve = twoRowDetail
			}
			avail := rowR.Dy() - detailReserve - SpaceSM
			if avail < rows { // pathological tiny panel
				avail = rows
			}
			// Height budget per strip row (band incl. its rowGap), fit `rows`.
			if perRow := (avail - rowGap*(rows-1)) / rows; perRow < stripRowH {
				stripRowH = perRow
				if stripRowH < 12 {
					stripRowH = 12
				}
				if chipH > stripRowH {
					chipH = stripRowH
				}
				chipInset = (stripRowH - chipH) / 2
				if chipInset < 0 {
					chipInset = 0
				}
			}
		}
		for i := range sections {
			w := synthChipIdealWidth(sections[i].id, chipMinW)
			if w > rowR.Dx() {
				w = rowR.Dx()
			}
			if x+w > rowR.Max.X && x > rowR.Min.X {
				x = rowR.Min.X
				y += stripRowH + rowGap
			}
			dv.appendSynthChip(sections[i], image.Rect(x, y+chipInset, x+w, y+chipInset+chipH), instID, selID)
			x += w + gap
		}
	} else {
		// Desktop: single connected row. Chips share the width equally,
		// clamped to [chipMinW, 2×chipMinW] so a short pipeline doesn't
		// balloon and a long one truncates labels instead of clipping.
		w := (rowR.Dx() - gap*(n-1)) / n
		if w < chipMinW {
			w = chipMinW
		}
		if maxW := chipMinW * 2; w > maxW {
			w = maxW
		}
		for i := range sections {
			dv.appendSynthChip(sections[i], image.Rect(x, y+chipInset, x+w, y+chipInset+chipH), instID, selID)
			x += w + gap
		}
	}
	stripBottom := y + stripRowH

	detailR := image.Rectangle{}
	if top := stripBottom + SpaceSM; top < rowR.Max.Y {
		detailR = image.Rect(rowR.Min.X, top, rowR.Max.X, rowR.Max.Y)
	}
	if mobile && !detailR.Empty() {
		// Right column for the stacked waves (Your sound + focus graph); the knob
		// grid keeps the left column at full body height. Width ~43% of the detail
		// body, clamped so the knob column keeps room for one good-sized dial +
		// label and the wave column stays legible. If the body is too narrow for
		// both, skip the split (knobs take the whole body; waves omitted at that
		// pathological width).
		//
		// DELIBERATE (re-confirmed 2026-07-04 design pass): a full-width bottom
		// band was prototyped instead and reverted — vertical space is the
		// scarce axis on phones, and stacking broke the mobile guarantees
		// pinned by synth_mobile_focus_test.go (waves visible at 390x720,
		// >= 2 knob rows at 390x896, ideal dial size at 390x844). The
		// side-by-side split preserves all of them by spending width.
		const minKnobColW, minWaveColW = 150, 110
		waveW := detailR.Dx() * 43 / 100
		if waveW > detailR.Dx()-minKnobColW {
			waveW = detailR.Dx() - minKnobColW
		}
		if waveW >= minWaveColW {
			dv.instEditorMobileFocusRect = image.Rect(
				detailR.Max.X-waveW, detailR.Min.Y, detailR.Max.X, detailR.Max.Y,
			)
			detailR.Max.X = dv.instEditorMobileFocusRect.Min.X - SpaceSM
		}
	}
	dv.instEditorDetailR = detailR
	// Effective detail-header height: the full token when the pane is tall
	// enough for header + one ideal knob row; otherwise shrink toward the
	// bare title row so the knobs (the actual controls) keep their space.
	headerH := dv2.SynthDetailHeaderH
	if knobRowMin := dv2.SynthControlMin + dv2.SynthControlCaptionH + 2*synthSectionPaddingY; detailR.Dy() < headerH+knobRowMin {
		headerH = detailR.Dy() - knobRowMin
		if headerH < synthSectionTitleHeight {
			headerH = synthSectionTitleHeight
		}
	}
	dv.instEditorDetailHeaderH = headerH

	for i := range sections {
		if sections[i].id == selID {
			sections[i].rect = detailR
		} else {
			sections[i].rect = image.Rectangle{}
		}
		dv.placeKnobsInSection(sections[i], mobile)
	}
}

// appendSynthChip records one chip in the strip model (reused slice — no
// per-frame allocation once capacity is established).
func (p *synthPanel) appendSynthChip(s synthSection, r image.Rectangle, instID string, selID synthSectionID) {
	dv := p.dv

	enabled := true
	if s.enableParam != "" {
		enabled = synthStageEnabled(instID, s.enableParam)
	}
	dv.instEditorChips = append(dv.instEditorChips, synthChip{
		id:          s.id,
		rect:        r,
		enableParam: s.enableParam,
		enabled:     enabled,
		selected:    s.id == selID,
	})
}

// synthChipWrapRows counts how many rows the greedy mobile wrap (the loop in
// layoutSynthSections) would use for `sections` at the given chip metrics. Kept
// in lock-step with that loop's x-advance/wrap rule so the detail-floor compaction
// reserves the right amount of strip height.
func synthChipWrapRows(sections []synthSection, rowR image.Rectangle, chipMinW, gap int) int {
	if len(sections) == 0 {
		return 0
	}
	rows := 1
	x := rowR.Min.X
	for i := range sections {
		w := synthChipIdealWidth(sections[i].id, chipMinW)
		if w > rowR.Dx() {
			w = rowR.Dx()
		}
		if x+w > rowR.Max.X && x > rowR.Min.X {
			x = rowR.Min.X
			rows++
		}
		x += w + gap
	}
	return rows
}

// synthChipStateW reserves room inside a chip for the enabled/ghost state
// affordance (dot / power ring) right of the label.
const synthChipStateW = 12

// synthChipIdealWidth is the measured chip width for a stage label at
// caption scale, floored at the density minimum (mobile wrap path).
func synthChipIdealWidth(id synthSectionID, minW int) int {
	captionScale := FontSizeCaption / FontSizeBody
	w := int(float64(TextWidth(sectionLabelLocalized(id)))*captionScale) + 2*SpaceSM + synthChipStateW
	if w < minW {
		w = minW
	}
	return w
}

// sectionGrid returns the persistent ControlGrid for a section id, creating it
// on first use. Keyed by id (not slice position) so the scroll position
// survives a recipe switch that reorders the sections.
func (p *synthPanel) sectionGrid(id synthSectionID) *ControlGrid {
	dv := p.dv

	if dv.instEditorSectionGrids == nil {
		dv.instEditorSectionGrids = map[synthSectionID]*ControlGrid{}
	}
	g := dv.instEditorSectionGrids[id]
	if g == nil {
		g = NewControlGrid(controlGridScrollbarStyle())
		dv.instEditorSectionGrids[id] = g
	}
	return g
}

// synthTabUpdate ticks the per-frame scroll cooldown clock for every section
// grid (see ControlGrid.WheelStep / Tick). Also drives the shared numeric
// editor (paramEditor) so blur-to-commit works in production (mirrors
// TransportZone.Update pumping the BPM box every frame). Returns true when
// the layout should be re-derived (e.g. editor opened/closed).
func (p *synthPanel) synthTabUpdate() bool {
	dv := p.dv

	// Advance the desktop wheel-nudge debounce; a spin that has paused for
	// wheelNudgeCommitFrames commits its single undo step here.
	dv.synthWheelNudge.tick()

	for _, g := range dv.instEditorSectionGrids {
		if g != nil {
			g.Tick()
		}
	}
	if dv.paramEditor != nil {
		wasActive := dv.paramEditor.Active()
		dv.paramEditor.Update()
		// If the editor just closed (commit on blur), request a re-layout so
		// the knob caption refreshes with the new value.
		if wasActive && !dv.paramEditor.Active() {
			return true
		}
	}
	// Stage 4: a completed async mirror render flips ready; consume it (one-shot)
	// and request a redraw so the freshly rendered note is shown promptly.
	if dv.synthMirror != nil && dv.synthMirror.consumeReady() {
		// The full-note card's render key (synth_full_note.go) can be
		// unchanged across this event — updateSynthFullNoteLive's async job
		// completing doesn't bump the params hash, only the ghost flag flips
		// (once, at request time, not at completion). Force both trace
		// sprites to re-render next Draw so a freshly-landed async render (or
		// the steady-wave slot) is never blitted stale from cache.
		p.sprites.fullNote.Invalidate()
		p.sprites.mirror.Invalidate()
		return true
	}
	return false
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

func (p *synthPanel) placeKnobsInSection(s synthSection, mobile bool) {
	dv := p.dv

	grid := dv.sectionGrid(s.id)
	// Collapsed stage (empty section rect) or no knobs: clear every knob
	// rect explicitly. Beware image.Rect's coordinate normalization — deriving
	// innerR from an empty s.rect would swap the negative spans into a small
	// NON-empty rect near the origin and leak phantom knob rects.
	if len(s.knobIdxs) == 0 || s.rect.Empty() {
		grid.Layout(image.Rectangle{}, 0, 1, 1, 0, 0)
		for _, kIdx := range s.knobIdxs {
			if kIdx < 0 || kIdx >= len(dv.instEditorControls) {
				continue
			}
			dv.instEditorControls[kIdx].SetRect(image.Rectangle{})
			dv.instEditorSliders[kIdx].SetRect(image.Rectangle{})
		}
		return
	}
	// The laid-out section IS the detail pane (collapsed sections arrive
	// with an empty rect). Its header band hosts title + plain-English
	// subtitle + the enable pill side by side — taller than the old
	// per-card title row, which is what ended the pill-overlaps-knobs era.
	innerR := image.Rect(
		s.rect.Min.X+synthSectionPaddingX,
		s.rect.Min.Y+dv.instEditorDetailHeaderH+synthSectionPaddingY,
		s.rect.Max.X-synthSectionPaddingX,
		s.rect.Max.Y-synthSectionPaddingY,
	)
	if innerR.Empty() {
		grid.Layout(image.Rectangle{}, 0, 1, 1, 0, 0)
		return
	}
	dv2 := Profile().DensityValues()
	hGap := SpaceSM
	vGap := SpaceXS
	// Cell height tracks the ideal knob + caption + a step-badge band below the
	// caption (continuous knobs draw their resolution pill there; reserving the
	// band uniformly keeps the grid even and stops the badge from being clamped
	// up onto the caption text). Shrinks to the card when even one ideal row
	// won't fit — so short cards degrade gracefully (knob scales down) instead
	// of overflowing, matching the pre-grid sizing.
	// Cell height = dial + caption band + concept-viz band (the abstract
	// "what this knob does" picture drawn under the caption, see
	// drawSynthDetailPane). The resolution badge is laid out INLINE on the
	// caption row, so it needs no extra vertical space; the viz band is the
	// only growth over the original sizing. The dial diameter is still capped
	// to SynthControlIdeal and placed at the TOP of the slot, so the taller cell
	// does NOT enlarge the dial or change its hit geometry — it just frees the
	// lower slot for the viz band, leaving left/right knob drag unaffected.
	// Shrinks to the card when even one ideal row won't fit.
	// The plain-English purpose gloss ("bright or dull", …) is drawn on the
	// caption ROW beside the value (see drawSynthDetailPane / synthControlPurpose) —
	// the fewer-per-row cap makes cells wide enough for both — so it costs no
	// vertical space here; the cell only reserves dial + caption + concept band.
	cellH := synthControlCellHeight()
	if availH := innerR.Dy(); cellH > availH {
		cellH = availH
	}
	if cellH < 1 {
		cellH = 1
	}
	// Card cells flow as many columns as fit the pane at the ParamCard ideal
	// width (both platforms). Mobile self-limits because the pane is narrower,
	// not because of an explicit column cap. Overflow scrolls — the intended
	// "show a few, scroll for more" behaviour.
	grid.SetMaxCols(synthMaxGridCols)

	// P4 tiering: the stage's Essential (character) knobs always come first; its
	// Advanced (fine-tuning) knobs are laid out below an always-visible in-grid
	// divider row. The divider is a full phantom row (knob idx -1) so essentials
	// round up to a full row, the divider band spans its own row, and advanced
	// knobs start fresh below it. No expander, no hidden state — advanced knobs
	// may only scroll out of the window like any other cell.
	essential, advanced := dv.synthSectionTierSplit(s.knobIdxs)
	dv.synthAdvDividerRect = image.Rectangle{}
	dv.synthAdvDividerCount = len(advanced)
	visible := essential
	dividerStart, dividerEnd := -1, -1
	if len(advanced) > 0 {
		// Two-pass: the first Layout discovers the column count for this width,
		// then the padded list (essentials rounded up to a full row + one
		// phantom divider row + advanced) is laid out for real. Probe with a
		// count of at least synthMaxGridCols so the grid's `cols > count` clamp
		// can't report fewer columns than the real (padded) list will use — that
		// mismatch would place the divider phantoms on the essentials' row. Save
		// and restore the scroll offset around the probe: the probe's smaller row
		// count would otherwise clamp the scroll window DOWN, and the real Layout
		// preserves whatever offset it finds — scrolling the advanced tier out of
		// view.
		savedFirst := grid.Scroll().VS.First
		probeCount := len(essential) + len(advanced)
		if probeCount < synthMaxGridCols {
			probeCount = synthMaxGridCols
		}
		grid.Layout(innerR, probeCount, dv2.ParamCardW, cellH, hGap, vGap)
		cols := grid.Cols()
		if cols < 1 {
			cols = 1
		}
		grid.Scroll().VS.First = savedFirst
		padded := append([]int{}, essential...)
		for len(padded)%cols != 0 {
			padded = append(padded, -1)
		}
		dividerStart = len(padded)
		for i := 0; i < cols; i++ { // one full phantom row = the divider band
			padded = append(padded, -1)
		}
		dividerEnd = len(padded)
		visible = append(padded, advanced...)
	}
	grid.Layout(innerR, len(visible), dv2.ParamCardW, cellH, hGap, vGap)
	if dividerStart >= 0 {
		var dr image.Rectangle
		for i := dividerStart; i < dividerEnd; i++ {
			if c, vis := grid.CellRect(i); vis {
				dr = dr.Union(c)
			}
		}
		dv.synthAdvDividerRect = dr
	}

	for i, kIdx := range visible {
		if kIdx < 0 {
			continue // phantom (padding / divider) cell — no widget
		}
		slot, vis := grid.CellRect(i)
		if !vis {
			dv.instEditorControls[kIdx].SetRect(image.Rectangle{})
			dv.instEditorSliders[kIdx].SetRect(image.Rectangle{})
			continue
		}
		// The card fills the whole grid cell. Task 4 swaps the dial DRAW for the
		// ParamCard renderer against this same rect; the slider mirrors the knob
		// rect so legacy hit-tests + JS export rects keep landing on the widget.
		dv.instEditorControls[kIdx].SetRect(slot)
		dv.instEditorSliders[kIdx].SetRect(slot)
	}
}

// drawSynthTab renders the prebuilt section model. Called by EQPanelZone.Draw
// when activeTab == TabSynth.
func (p *synthPanel) drawSynthTab(dst *ebiten.Image, contentR image.Rectangle, instID string) {
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
	// Task 15: refresh the per-chip stage thumbnails (hash-gated) before the
	// chip strip draws them as faint waveform watermarks.
	dv.ensureStageThumbs(resolved)
	dv.drawSynthChipStrip(dst)
	dv.drawSynthDetailPane(dst, resolved)
	// Numeric editor overlay: floats above the detail pane, anchored over the
	// tapped knob caption. Drawn before the preview pane / footer / save-as
	// dialog so it sits above detail-pane content but below modal dialogs.
	if dv.paramEditor != nil {
		dv.paramEditor.Draw(dst)
	}
	// Phase 4 audio-panel redesign: right-half preview pane (osc +
	// ADSR + filter) when the layout reserved space for it. Reads
	// the row's decay knob value via the synth bindings.
	if r := dv.instEditorPreviewRect; !r.Empty() {
		// "Your sound": the live wave-shape mirror owns the whole right pane.
		// It re-renders in real time as knobs move (updateSynthMirrorLive is
		// hash-gated) and shows a short, legible window of the actual waveform.
		// The old stacked OSC/ADSR/FILTER seed plots + OUT-row meters were
		// removed — the per-knob concept pictures already teach those, and the
		// extra strips just cluttered the pane.
		dv.updateSynthMirrorLive(resolved)
		dv.updateSynthFullNoteLive(resolved)
		fullR, upR, focusR := splitSynthRightPane3(r)
		if !fullR.Empty() {
			p.drawSynthFullNote(dst, fullR, resolved, dv.synthMirror)
		}
		if !upR.Empty() {
			p.drawSynthMirror(dst, upR, resolved, dv.synthMirror)
		}
		dv.drawSynthFocusGraph(dst, focusR, resolved)
	}
	// Mobile: no side preview pane — the "Your sound" full-note + steady-wave mirror
	// + focus graph live in a RIGHT column beside the knob grid
	// (instEditorMobileFocusRect), split vertically into three cards (full note /
	// up close / focus graph) by splitSynthRightPane3.
	if mb := dv.instEditorMobileFocusRect; !mb.Empty() {
		dv.updateSynthMirrorLive(resolved)
		dv.updateSynthFullNoteLive(resolved)
		fullR, upR, focusR := splitSynthRightPane3(mb)
		if !fullR.Empty() {
			p.drawSynthFullNote(dst, fullR, resolved, dv.synthMirror)
		}
		if !upR.Empty() {
			p.drawSynthMirror(dst, upR, resolved, dv.synthMirror)
		}
		dv.drawSynthFocusGraph(dst, focusR, resolved)
	}
	dv.drawSynthFooter(dst)
	dv.drawSaveAsDialog(dst)
}

// synthDecayValueForPreview reads the current decay-knob normalized
// value out of the active bindings and converts it to a multiplier
// (1.0 = no change, 0.25 = quarter, 4.0 = quadruple). Returns 1.0
// when no decay binding is present so the ADSR plot stays meaningful.
func (p *synthPanel) synthDecayValueForPreview() float64 {
	dv := p.dv

	for i, b := range dv.instEditorBindings {
		if b.def.Name != "decay" || i >= len(dv.instEditorControls) {
			continue
		}
		k := dv.instEditorControls[i]
		if k == nil {
			continue
		}
		denom := b.def.Max - b.def.Min
		if denom <= 0 {
			return 1.0
		}
		v := b.def.Min + k.Value*denom
		if v <= 0 {
			return 1.0
		}
		return v
	}
	return 1.0
}

// drawSynthOutRow paints the per-row OUT strip at the bottom of the
// preview pane: Delay send, Reverb send, Channel volume — each as a
// labelled mini-meter so the user can see whether their knob changes
// reach the master.
func (p *synthPanel) drawSynthOutRow(dst *ebiten.Image, paneR image.Rectangle, instID string) {
	dv := p.dv
	_ = dv

	const rowH = 20
	if paneR.Dy() < 80 {
		return
	}
	rowR := image.Rect(paneR.Min.X+SpaceSM, paneR.Max.Y-rowH-SpaceSM, paneR.Max.X-SpaceSM, paneR.Max.Y-SpaceSM)
	drawRect(dst, rowR, WithAlpha(genColorVizScopeBg, AlphaOverlay), true)

	captionH := StyledTextHeight(RoleMicro)
	colW := rowR.Dx() / 3
	for i, item := range []struct {
		label string
		v     float64
	}{
		{"DELAY", audio.DelaySend(instID)},
		{"REVERB", audio.ReverbSend(instID)},
		{"VOL", audio.ChannelVolume(instID)},
	} {
		cellR := image.Rect(rowR.Min.X+i*colW, rowR.Min.Y, rowR.Min.X+(i+1)*colW, rowR.Max.Y)
		// Label.
		DrawTextStyled(dst, item.label, cellR.Min.X+2, cellR.Min.Y+1, RoleMicro, TokenTextSecondary())
		// Mini bar: width proportional to value (clamped 0..1).
		barTopY := cellR.Min.Y + captionH + 2
		barR := image.Rect(cellR.Min.X+2, barTopY, cellR.Max.X-2, cellR.Max.Y-2)
		drawRect(dst, barR, WithAlpha(genColorBorder, AlphaSubtle), true)
		frac := item.v
		if frac < 0 {
			frac = 0
		} else if frac > 1 {
			frac = 1
		}
		if frac > 0 {
			fillR := image.Rect(barR.Min.X, barR.Min.Y, barR.Min.X+int(frac*float64(barR.Dx())), barR.Max.Y)
			drawRect(dst, fillR, WithAlpha(colAccent, AlphaStrong), true)
		}
	}
}

// drawSynthHeader renders the instrument/recipe caption and the cached
// waveform thumbnail. No retrigger or vol pill — the row rack owns the
// per-row volume slider and the transport bar owns Play.
func (p *synthPanel) drawSynthHeader(dst *ebiten.Image) {
	dv := p.dv

	h := dv.instEditorHeader
	if h.rect.Empty() {
		return
	}
	// Background card.
	drawRoundedRect(dst, h.rect, TokenSurface1(), RadiusSM, true)
	drawRoundedRect(dst, h.rect, TokenBorderSubtle(), RadiusSM, false)

	// Caption: "snare  —  drum-snare", truncated to its rect so it never
	// runs under the Save/Save As/Reset buttons on a narrow (mobile) header.
	DrawTextStyled(dst, synthHeaderCaptionText(h), h.captionRect.Min.X, h.captionRect.Min.Y+SpaceSM, RoleSectionHeader, TokenTextPrimary())

	// Cached-source mini-waveform. When the voice cache holds a real
	// rendered sample for this instrument, render it via the same
	// min/max-per-pixel-column engine the Wave tab uses. Otherwise fall
	// back to the symmetric exponential placeholder so the panel never
	// shows a blank box.
	drawSynthHeaderWaveform(dst, h.waveformRect, h.instLabel)
}

// synthHeaderCaptionText builds the "inst — recipe" caption and truncates it
// to the header's caption rect so it never overflows into the action buttons
// (the mobile caption-overflow bug). Reuses the shared truncCaption helper.
func synthHeaderCaptionText(h synthHeaderLayout) string {
	caption := h.instLabel
	// Suppress the recipe segment when it duplicates the instrument id (e.g.
	// a user recipe whose id equals the instrument) so the header never reads
	// "fm-bell — fm-bell".
	if h.recipeID != "" && h.recipeID != h.instLabel {
		caption = h.instLabel + "  —  " + h.recipeID
	}
	// Styled metric: the caption is drawn at RoleSectionHeader, whose
	// SemiBold glyphs out-measure the body TextWidth — body-metric
	// truncation let long captions run under the Preview button.
	return truncCaptionStyled(caption, h.captionRect.Dx(), RoleSectionHeader)
}

// testSynthHeaderSampleOverride lets tests inject a fake "cached
// sample" buffer without driving the real voice cache. Production
// builds leave it nil and the renderer consults audio.LatestVoiceSample.
var testSynthHeaderSampleOverride func(instrumentID string) []float32

// minSynthHeaderSamples is the minimum buffer length before the real
// cached sample replaces the placeholder. Below this, min/max-per-pixel
// downsampling collapses into a near-flat line that reads as a glitch
// rather than a waveform — better to keep the recognisable placeholder.
const minSynthHeaderSamples = 16

// drawSynthHeaderWaveform paints the Synth-tab header mini-waveform.
// Prefers the real cached sample (from the voice cache) when available,
// falls back to drawSynthWaveformPlaceholder otherwise. Replacing the
// placeholder when real audio data exists turns the dead label-only
// header into a live "this is the source you're shaping" preview.
// Phase 4.
func drawSynthHeaderWaveform(dst *ebiten.Image, r image.Rectangle, instID string) {
	if r.Empty() {
		return
	}
	// Background card.
	drawRoundedRect(dst, r, TokenSurface2(), RadiusXS, true)
	drawRoundedRect(dst, r, TokenBorderSubtle(), RadiusXS, false)

	var sample []float32
	if testSynthHeaderSampleOverride != nil {
		sample = testSynthHeaderSampleOverride(instID)
	} else if instID != "" {
		sample = audio.LatestVoiceSample(instID)
	}

	if len(sample) >= minSynthHeaderSamples {
		// Convert float32 sample to float64 for drawWaveTrace, downsample
		// to at most one value per pixel column so we don't allocate a
		// massive intermediate.
		width := r.Dx() - 4
		if width < 8 {
			return
		}
		midY := r.Min.Y + r.Dy()/2
		inner := image.Rect(r.Min.X+2, r.Min.Y+2, r.Min.X+2+width, r.Max.Y-2)
		drawWaveTrace(dst, synthHeaderWaveFloat64(sample), inner, midY, width, colWaveTrace, 1.0, nil)
		return
	}

	// Fallback placeholder. The placeholder redraws the bg/border which
	// is idempotent — no visual penalty.
	drawSynthWaveformPlaceholder(dst, r)
}

// synthHeaderWaveScratch is the reusable float64 conversion buffer for the
// header mini-waveform. The synth-tab Draw path is single-threaded (Ebiten
// Draw), so a package-level scratch avoids a per-frame allocation without a
// lock. Sized up monotonically; never shrinks.
var synthHeaderWaveScratch []float64

// synthHeaderWaveFloat64 converts the float32 sample to float64 for
// drawWaveTrace, reusing synthHeaderWaveScratch so the header Draw allocates
// nothing once the buffer length stabilises.
func synthHeaderWaveFloat64(sample []float32) []float64 {
	if cap(synthHeaderWaveScratch) < len(sample) {
		synthHeaderWaveScratch = make([]float64, len(sample))
	}
	synthHeaderWaveScratch = synthHeaderWaveScratch[:len(sample)]
	for i, v := range sample {
		synthHeaderWaveScratch[i] = float64(v)
	}
	return synthHeaderWaveScratch
}

// drawSynthWaveformPlaceholder renders a static symmetric envelope as a
// placeholder for the cached waveform. Used directly by tests; the
// production header renderer (drawSynthHeaderWaveform) wraps this with
// real-sample fallback. The placeholder communicates "this is the
// source for the active row" without depending on the cache wiring.
func drawSynthWaveformPlaceholder(dst *ebiten.Image, r image.Rectangle) {
	if r.Empty() {
		return
	}
	drawRoundedRect(dst, r, TokenSurface2(), RadiusXS, true)
	drawRoundedRect(dst, r, TokenBorderSubtle(), RadiusXS, false)
	// Stroke a symmetric exponential decay envelope. The drum-bus
	// inspirations (Drum Bus, Drum Synth) all show a similar gesture in
	// their header.
	mid := (r.Min.Y + r.Max.Y) / 2
	w := r.Dx()
	if w < 8 {
		return
	}
	col := TokenAccentDim()
	for x := 0; x < w; x++ {
		// Exponential decay over the width.
		t := float64(x) / float64(w)
		amp := float64(r.Dy()/2-2) * expDecay(t*4)
		y0 := mid - int(amp)
		y1 := mid + int(amp)
		drawRect(dst, image.Rect(r.Min.X+x, y0, r.Min.X+x+1, y1), col, true)
	}
}

// expDecay is a small inline exp(-x) shim that avoids dragging in math
// inside the placeholder draw path (which runs every frame the synth tab
// is visible).
func expDecay(x float64) float64 {
	// 8-term Taylor expansion is plenty for x∈[0,4] visual precision.
	sum, term := 1.0, 1.0
	for i := 1; i < 12; i++ {
		term *= -x / float64(i)
		sum += term
	}
	if sum < 0 {
		return 0
	}
	if sum > 1 {
		return 1
	}
	return sum
}

// drawSynthSectionCard renders one section card. Empty sections collapse
// to a placeholder showing the section name + dash, so the layout grid
// stays visually consistent across recipes with very different wired sets.
// chipStripBounds returns the union of every non-empty
// dv.instEditorChips[i].rect — the strip's TraceSprite cache rect. Zero value
// (Empty()==true) when no chips are laid out yet (pre-Layout frame) or every
// chip collapsed to an empty rect (drawSynthChipStrip returns early on that).
func (p *synthPanel) chipStripBounds() image.Rectangle {
	dv := p.dv

	var strip image.Rectangle
	first := true
	for _, c := range dv.instEditorChips {
		if c.rect.Empty() {
			continue
		}
		if first {
			strip = c.rect
			first = false
			continue
		}
		strip = strip.Union(c.rect)
	}
	return strip
}

// chipEnableHash folds every chip's (id, enabled, selected) triple into one
// value — the chip-strip cache key component covering "what state is each
// pill in" (fill/border/label-color/state-dot/disabled-watermark-line all
// derive from these two bools per chip). Hand-rolled FNV-ish mix instead of
// fmt/hash-fnv: this runs every Draw call (steady frames too, to detect a
// key change), so it must stay allocation-free.
func (p *synthPanel) chipEnableHash() uint64 {
	dv := p.dv

	const offset64 uint64 = 14695981039346656037
	const prime64 uint64 = 1099511628211
	h := offset64
	for _, c := range dv.instEditorChips {
		h ^= uint64(c.id)
		h *= prime64
		if c.enabled {
			h ^= 1
		}
		h *= prime64
		if c.selected {
			h ^= 2
		}
		h *= prime64
	}
	return h
}

// drawSynthChipStrip paints the pipeline chip strip: one compact chip per
// stage in audio order, connected by a wire so the strip reads as the signal
// path. Selected chip = accent border + tinted fill; disabled stage = dimmed
// ghost chip with a power-ring affordance (IconCircle outline). Connectors
// are drawn geometry — the forbidden-glyph rule bans `→`/`▶` text.
//
// TraceSprite-cached (Task 4 of the render-caching plan): the chip rects
// come from Layout (dv.instEditorChips[i].rect), NOT from this draw, so
// hit-testing is unaffected by caching the pixels. The strip's own bounding
// rect (chipStripBounds) is the sprite rect passed to TraceSprite.Draw.
// Unlike the focus/mirror/full-note adopters (whose content functions only
// ever consult the rect THEY were called with), the chip strip's absolute
// positions live in dv.instEditorChips[i].rect — fixed by Layout,
// independent of whatever rect this draw is asked to fill. TraceSprite.Draw
// hands the render callback (img, r) where r is image.Rect(0,0,w,h) in the
// cached branch but the ORIGINAL `strip` rect (unchanged) in the RENDER_SAFE
// bypass branch (see trace_sprite.go). origin = strip.Min - r.Min is
// therefore strip.Min when cached (shifting the absolute chip rects down to
// the (0,0)-anchored offscreen texture) and the zero point under
// RENDER_SAFE (r.Min == strip.Min already, so chips draw at their true
// absolute position on the real dst — required, since other UI layers drawn
// before/after this one are NOT shifted).
func (p *synthPanel) drawSynthChipStrip(dst *ebiten.Image) {
	dv := p.dv

	strip := p.chipStripBounds()
	if strip.Empty() {
		return
	}
	sel := synthSectionID(-1)
	if s := p.synthSelectedSection(); s != nil {
		sel = s.id
	}
	key := struct {
		sel    synthSectionID
		enable uint64
		thumb  int
		w, h   int
	}{sel, p.chipEnableHash(), dv.lazyStageThumbs().contentVersion(), strip.Dx(), strip.Dy()}
	p.sprites.chip.Draw(dst, strip, key, func(img *ebiten.Image, r image.Rectangle) {
		p.drawSynthChipStripContent(img, strip.Min.Sub(r.Min))
	})
}

// drawSynthChipStripContent is the pixel body of drawSynthChipStrip,
// factored out so it can be handed to TraceSprite.Draw's render callback.
// origin is subtracted from EVERY absolute chip/connector/watermark/label
// coordinate before it's drawn, so the same code paints correctly whether
// dst is the real screen (origin == image.Point{}, RENDER_SAFE bypass) or a
// strip-sized offscreen texture anchored at (0,0) (origin == strip.Min, the
// cached path). Comparisons that decide layout (gap width, same-row check)
// use the UNTRANSLATED chip rects — subtracting a constant from both sides
// of a comparison never changes its outcome, and reusing the original rects
// there keeps this diff-minimal against the pre-cache version.
func (p *synthPanel) drawSynthChipStripContent(dst *ebiten.Image, origin image.Point) {
	dv := p.dv

	for i, c := range dv.instEditorChips {
		if c.rect.Empty() {
			continue
		}
		cr := c.rect.Sub(origin)
		// Connector: an accent chevron centred in the inter-chip gap so the
		// strip reads as a directed signal path. When the gap is too tight for
		// a legible arrow (wrapped mobile rows) fall back to the old thin wire.
		// Drawn BEFORE the chip body so the pill always sits on top.
		if i > 0 {
			prevChip := dv.instEditorchipPrev(i)
			if !prevChip.rect.Empty() && prevChip.rect.Min.Y == c.rect.Min.Y && prevChip.rect.Max.X < c.rect.Min.X {
				pr := prevChip.rect.Sub(origin)
				cy := (cr.Min.Y + cr.Max.Y) / 2
				gapW := cr.Min.X - pr.Max.X
				if gapW >= 4 {
					side := gapW
					if side > 10 {
						side = 10
					}
					gapCx := (pr.Max.X + cr.Min.X) / 2
					arrowR := image.Rect(
						gapCx-side/2, cy-side/2,
						gapCx-side/2+side, cy-side/2+side,
					)
					DrawIcon(dst, IconChevronRight, arrowR, WithAlpha(colAccent, AlphaMedium))
				} else {
					wire := image.Rect(pr.Max.X, cy, cr.Min.X, cy+1)
					drawRect(dst, wire, WithAlpha(genColorBorder, AlphaFaint), true)
				}
			}
		}
		// Chip body. Every chip keeps a pill background and border so a
		// disabled chip never reads as a floating bare label — disabled gets a
		// dimmer fill + muted label, selected gets the accent treatment.
		//
		// Four visual states, three of which can combine:
		//   enabled  + unselected → surface pill, secondary label
		//   disabled + unselected → dimmer pill, disabled label
		//   enabled  + selected   → accent fill + full accent ring, primary label
		//   disabled + selected   → accent fill + HALF-alpha accent ring, DIMMED
		//                            label (the ghost-but-open read).
		var fill color.Color = TokenSurface1()
		var labelCol color.Color = TokenTextSecondary()
		var border color.Color = TokenBorderSubtle()
		if !c.enabled {
			fill = WithAlpha(TokenSurface2(), AlphaStrong)
			labelCol = TokenTextDisabled()
		}
		if c.selected {
			fill = WithAlpha(colAccent, AlphaFaint)
			if c.enabled {
				labelCol = TokenTextPrimary()
				border = colAccent
			} else {
				// Selected-but-disabled: the ring is half-strength and the
				// label stays dimmed so it never reads as a fully-active chip.
				labelCol = TokenTextDisabled()
				border = WithAlpha(colAccent, AlphaMedium)
			}
		}
		drawRoundedRect(dst, cr, fill, RadiusXS, true)
		drawRoundedRect(dst, cr, border, RadiusXS, false)
		// Waveform watermark: the stage's cumulative preview drawn as a subtle
		// min/max column band confined to the BOTTOM band of the chip, so it
		// never crosses the vertically-centred label (a full-height/centred
		// band read as strikethrough text on disabled chips). Disabled stages
		// get a flat 1px line at the band's centre instead (no signal to show).
		// Painted AFTER the pill, BEFORE the label/state dot, so the label
		// stays dominant. Zero layout impact — a watermark inside the chip rect.
		bandH := cr.Dy() * 2 / 5
		if bandH >= 4 {
			bandR := image.Rect(cr.Min.X+SpaceSM, cr.Max.Y-2-bandH, cr.Max.X-SpaceSM, cr.Max.Y-2)
			if bandR.Dx() > 1 && bandR.Dy() > 1 {
				bandMidY := (bandR.Min.Y + bandR.Max.Y) / 2
				if !c.enabled {
					drawRect(dst, image.Rect(bandR.Min.X, bandMidY, bandR.Max.X, bandMidY+1), WithAlpha(TokenTextDisabled(), AlphaMedium), true)
				} else if wave := dv.stageThumbs.waveForRef(i); len(wave) > 0 { // no-copy read (read-only contract)
					mins, maxs := pcmMinMaxColumns(wave, bandR.Dx())
					half := float64(bandR.Dy()) / 2
					// Normalise amplitude like the full-note / mirror traces so a
					// quiet stage preview still reads as a wave rather than a flat
					// line — same mirrorTraceGain the other watermark renderers use.
					gain := mirrorTraceGain(wave)
					wm := WithAlpha(colWaveTrace, AlphaSubtle)
					for col := range mins {
						yHi := bandMidY - int(clampSignedUnit(maxs[col]*gain)*half)
						yLo := bandMidY - int(clampSignedUnit(mins[col]*gain)*half)
						if yLo <= yHi {
							yLo = yHi + 1
						}
						if yHi < bandR.Min.Y {
							yHi = bandR.Min.Y
						}
						if yLo > bandR.Max.Y {
							yLo = bandR.Max.Y
						}
						x := bandR.Min.X + col
						drawRect(dst, image.Rect(x, yHi, x+1, yLo), wm, true)
					}
				}
			}
		}
		// State dot right of the label (only for stages that carry an enable
		// toggle). >= 6px, explicit on/off colors: accent when on, disabled
		// surface-text when off.
		if c.enableParam != "" {
			const dotD = 7
			cy := (cr.Min.Y + cr.Max.Y) / 2
			dotR := image.Rect(
				cr.Max.X-SpaceSM-dotD, cy-dotD/2,
				cr.Max.X-SpaceSM, cy-dotD/2+dotD,
			)
			if c.enabled {
				DrawIcon(dst, IconCircle, dotR, colAccent)
			} else {
				DrawIcon(dst, IconCircle, dotR, TokenTextDisabled())
			}
		}
		// Label, truncated to the room left of the state affordance.
		availPx := cr.Dx() - 2*SpaceSM - synthChipStateW
		if availPx <= 0 {
			continue
		}
		label := sectionLabelChipFitted(c.id, availPx)
		labelH := StyledTextHeight(RoleCaption)
		ly := cr.Min.Y + (cr.Dy()-labelH)/2
		DrawTextStyled(dst, label, cr.Min.X+SpaceSM, ly, RoleCaption, labelCol)
	}
}

// instEditorchipPrev returns the chip immediately before index i, used by the
// chip-strip connector. Small helper so the connector logic reads clearly.
func (p *synthPanel) instEditorchipPrev(i int) synthChip {
	dv := p.dv

	return dv.instEditorChips[i-1]
}

// drawSynthDetailPane paints the expanded stage: a header band (title +
// plain-English subtitle + enable pill, now with room so nothing overlaps)
// over the full-size knob grid. The bypass scrim dims the knob area when the
// selected stage is toggled off — the pill stays bright so re-enabling is
// obvious; knobs remain interactive (tweaking a bypassed stage is allowed,
// it just has no effect until re-enabled).
func (p *synthPanel) drawSynthDetailPane(dst *ebiten.Image, instID string) {
	dv := p.dv

	detail := dv.instEditorDetailR
	sel := dv.synthSelectedSection()
	if detail.Empty() || sel == nil {
		return
	}
	drawRoundedRect(dst, detail, TokenSurface1(), RadiusSM, true)
	drawRoundedRect(dst, detail, TokenBorderSubtle(), RadiusSM, false)

	headerH := dv.instEditorDetailHeaderH
	enState := dv.synthStageEnableState(instID, sel)
	// A pilled stage that is currently OFF dims its title + tags "BYPASSED".
	// Locked (VOICE, sole-source OSC) and un-pilled stages read as on.
	stageEnabled := enState.kind == enableNone || enState.enabled

	tx := detail.Min.X + synthSectionPaddingX
	ty := detail.Min.Y + SpaceXS
	// Title shares the body's dim state: when the stage is bypassed the title
	// dims too (it stayed full-brightness before, contradicting the dimmed
	// knobs) and gains a small "BYPASSED" tag so the off-state is unmistakable.
	titleCol := TokenTextPrimary()
	if !stageEnabled {
		titleCol = TokenTextDisabled()
	}
	title := sectionLabelLocalized(sel.id)
	DrawTextStyled(dst, title, tx, ty, RoleBody, titleCol)
	if !stageEnabled {
		tagX := tx + StyledTextWidth(title, RoleBody) + SpaceSM
		DrawTextStyled(dst, i18n.T(i18n.KeyCapBypassed), tagX, ty+2, RoleMicro, TokenTextDisabled())
	}
	if sub := sectionSubtitle(sel.id); sub != "" {
		subY := ty + StyledTextHeight(RoleBody) + 1
		if subY+StyledTextHeight(RoleCaption) <= detail.Min.Y+headerH {
			availW := detail.Dx() - 2*synthSectionPaddingX - FXToggleTrackW() - 2*SpaceXS
			sub = truncCaptionStyled(sub, availW, RoleCaption)
			subCol := WithAlpha(TokenTextSecondary(), AlphaMedium)
			if !stageEnabled {
				subCol = WithAlpha(TokenTextDisabled(), AlphaMedium)
			}
			DrawTextStyled(dst, sub, tx, subY, RoleCaption, subCol)
		}
	}
	if synthSectionHasPill(sel.id, sel.enableParam) {
		if pillR := dv.synthDetailEnablePillRect(); !pillR.Empty() {
			if enState.interactive {
				drawFXTogglePill(dst, pillR, enState.enabled)
			} else {
				// Mandatory / sole-source stage — locked on, greyed.
				drawFXTogglePillDisabled(dst, pillR)
			}
		}
	}

	// BUG 2 fix: clear all badge rects before drawing the selected section.
	// Badges whose knobs are NOT in the currently-drawn section keep stale
	// rects from the previous draw; those stale rects occupy the same pixel
	// area the detail pane reuses for every section, so a tap on a B-section
	// knob was stolen by an A-section badge's ghost rect. Clearing here
	// guarantees only the currently-visible section's badges have non-empty
	// rects, and the hit-test in handleSynthTabInput therefore only fires for
	// the section that is actually on screen.
	for _, b := range dv.instEditorStepBadges {
		if b != nil {
			b.SetRect(image.Rectangle{})
		}
	}
	// Mirror the badge-rect clear for readout rects: stale rects from a
	// previous section draw would let a readout tap misfire on an off-screen
	// knob. Reset all to empty; the draw loop below re-populates only the
	// knobs that are actually visible in this section.
	for i := range dv.instEditorReadoutRects {
		dv.instEditorReadoutRects[i] = image.Rectangle{}
	}

	// Every visible knob renders as a single ParamCard on ALL platforms: name,
	// value (enum/resolution-aware), and a range gauge in one keycap-style
	// control. Replaces the old dial/pill + separate caption + gloss + step
	// badge stack — the card's rim carries the selection ring (paramCardState.
	// Selected) and its gauge carries what the step badge used to show.
	p.sprites.beginCardFrame()
	for _, kIdx := range sel.knobIdxs {
		if kIdx < 0 || kIdx >= len(dv.instEditorControls) {
			continue
		}
		k := dv.instEditorControls[kIdx]
		r := k.Rect()
		if r.Empty() {
			continue // scrolled out of the detail pane's visible window
		}
		b := dv.instEditorBindings[kIdx]
		st := paramCardState{
			Selected: dv.synthSelectedControlIdx(instID, sel) == kIdx,
			Dim:      !stageEnabled,
		}
		name := synthParamDisplayName(b.def.Name)
		value := knobPillLabel(k, b.def)
		gauge := paramCardGaugeFor(k, b.def)
		key := struct {
			name, value string
			gauge       paramCardGauge
			st          paramCardState
		}{name, value, gauge, st}
		p.sprites.card(b.def.Name).Draw(dst, r, key, func(img *ebiten.Image, rr image.Rectangle) {
			drawParamCard(img, rr, name, value, gauge, st)
		})
	}
	p.sprites.endCardFrame()
	// Scrollbar + edge fades when the stage's knobs overflow the pane. The bottom
	// fade dissolves the last row into the reserved peek band — the "more below"
	// affordance; the top fade appears once scrolled down.
	stageGrid := dv.sectionGrid(sel.id)
	stageGrid.Draw(dst)
	drawControlGridEdgeFades(dst, stageGrid, TokenSurface1())
	p.drawSynthAdvDivider(dst)
	if sel.enableParam != "" && !stageEnabled {
		scrim := image.Rect(detail.Min.X+1, detail.Min.Y+headerH, detail.Max.X-1, detail.Max.Y-1)
		if !scrim.Empty() {
			drawRoundedRect(dst, scrim, WithAlpha(TokenSurface1(), AlphaStrong), RadiusXS, true)
		}
	}
}

// drawSynthAdvDivider renders the always-visible "— ADVANCED · N —" tier
// divider row (non-interactive). Empty rect (stage has no advanced params,
// or the row is scrolled out) draws nothing.
func (p *synthPanel) drawSynthAdvDivider(dst *ebiten.Image) {
	dv := p.dv
	r := dv.synthAdvDividerRect
	if r.Empty() {
		return
	}
	lbl := i18n.T(i18n.KeySynthAdvanced)
	if dv.synthAdvDividerCount > 0 {
		lbl += " · " + strconv.Itoa(dv.synthAdvDividerCount)
	}
	lw := StyledTextWidth(lbl, RoleMicro)
	lh := StyledTextHeight(RoleMicro)
	cx := r.Min.X + (r.Dx()-lw)/2
	cy := r.Min.Y + (r.Dy()-lh)/2
	hairY := r.Min.Y + r.Dy()/2
	hairCol := WithAlphaFromColor(TokenBorderSubtle(), AlphaMedium)
	if leftEnd := cx - SpaceSM; leftEnd > r.Min.X {
		drawRect(dst, image.Rect(r.Min.X, hairY, leftEnd, hairY+1), hairCol, true)
	}
	if rightStart := cx + lw + SpaceSM; rightStart < r.Max.X {
		drawRect(dst, image.Rect(rightStart, hairY, r.Max.X, hairY+1), hairCol, true)
	}
	DrawTextStyled(dst, lbl, cx, cy, RoleMicro, TokenTextSecondary())
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
		if btn == nil {
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
	case synthSaveButtonTag:
		return synthActionLabel("save")
	case synthSaveAsButtonTag:
		return synthActionLabel("save-as")
	case synthResetButtonTag:
		return synthActionLabel("reset")
	}
	return ""
}

// synthActionLabel returns the localized button label for a synth header
// action id ("preview"/"save"/"save-as"/"reset"). It is the single source of
// wording so the layout's width measurement and the draw path stay in sync,
// and so the wording can be shared with the Sampler tab. Labels are short
// enough that four of them fit a fixed full-width mobile action row without
// truncation in any supported language; should a future, longer translation
// not fit, abbreviate it here (one place) rather than reintroducing overflow.
func synthActionLabel(action string) string {
	switch action {
	case "preview":
		return i18n.T(i18n.KeyPreview)
	case "save":
		return i18n.T(i18n.KeySave)
	case "save-as":
		return i18n.T(i18n.KeySaveAs)
	case "reset":
		return i18n.T(i18n.KeyReset)
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
func (p *synthPanel) handleSynthTabInput(x, y int, pressed bool, instID string) bool {
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
	// Pipeline chips: a tap only OPENS (selects) the stage for inspection —
	// it never enables/disables it. Enabling a ghost (disabled) stage is an
	// explicit action via the detail-pane pill, so the user can look at a
	// disabled stage's knobs without turning it on.
	for _, c := range dv.instEditorChips {
		if c.rect.Empty() || !image.Pt(x, y).In(c.rect) {
			continue
		}
		dv.setSelectedSynthSection(instID, c.id)
		if dv.eqPanelZone != nil {
			dv.eqPanelZone.Invalidate()
		}
		return true
	}
	// Footer buttons (Reset / Save / Save-As).
	for _, btn := range dv.instEditorBtns {
		if !image.Pt(x, y).In(btn.Rect()) {
			continue
		}
		switch btn.Text {
		case synthResetButtonTag:
			dv.ResetActiveRecipe()
			return true
		case synthSaveButtonTag:
			dv.SaveActiveRecipe()
			return true
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

// ---- Readout-rect helpers + numeric editor ------------------------------

// synthControlReadoutRect returns the caption hit-rect for the given knob index.
// Returns the zero rectangle when the index is out of range or the rect has
// not been populated yet (no Draw pass has occurred).
func (p *synthPanel) synthControlReadoutRect(idx int) image.Rectangle {
	dv := p.dv

	if idx < 0 || idx >= len(dv.instEditorReadoutRects) {
		return image.Rectangle{}
	}
	return dv.instEditorReadoutRects[idx]
}

// setKnobReadoutRect stores the caption hit-rect for the given knob index,
// growing the slice as needed. Mirrors setKnobStepBadgeRect semantics from
// Task 11.
func (p *synthPanel) setKnobReadoutRect(idx int, r image.Rectangle) {
	dv := p.dv

	for len(dv.instEditorReadoutRects) <= idx {
		dv.instEditorReadoutRects = append(dv.instEditorReadoutRects, image.Rectangle{})
	}
	dv.instEditorReadoutRects[idx] = r
}

// openSynthParamEditor opens the shared numeric editor anchored over the
// caption (readout) of knob at idx. The editor's setValue callback writes
// back through the knob + propagate path so every existing audio write stays
// consistent.
func (p *synthPanel) openSynthParamEditor(idx int, instID string) {
	dv := p.dv

	if idx < 0 || idx >= len(dv.instEditorBindings) || idx >= len(dv.instEditorControls) {
		return
	}
	if dv.paramEditor == nil {
		dv.paramEditor = NewParamValueEditor()
	}
	def := dv.instEditorBindings[idx].def
	anchor := dv.synthControlReadoutRect(idx)
	dv.paramEditor.OpenValue(ValueOpen{
		Spec:          paramSpec(def),
		Anchor:        anchor,
		MobileInputID: "synth-param",
		Get: func() float64 {
			return def.Min + dv.instEditorControls[idx].Value*(def.Max-def.Min)
		},
		Set: func(v float64) {
			span := def.Max - def.Min
			if span <= 0 {
				span = 1
			}
			dv.instEditorControls[idx].Value = (v - def.Min) / span
			dv.syncSliderFromKnob(idx)
			dv.propagateSynthSliderValue(idx, instID)
			dv.requestSynthMirror(instID) // one-shot typed commit → refresh preview
			// A typed value is one discrete commit → one undo step.
			dv.commitInstrumentParams(instID)
		},
	})
}

// ---- Hit areas + propagate ---------------------------------------------

// propagateSynthSliderValue rescales a 0..1 slider value into the
// ParamDef's [Min, Max] range and pushes it through audio.SetInstrumentParam.
// Coalescing matches the pre-redesign behaviour: stationary drag bursts
// don't flood the JS bridge — see
// instrument_params_playback_stability.browser.test.js.
func (p *synthPanel) propagateSynthSliderValue(idx int, instID string) {
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
var synthAuditionFn = func(instID string) {
	audio.Play(instID)
}

// SwapSynthAuditionFnForTest installs a test fake for the audition trigger
// and returns the previous value. Mirrors the SwapPlatformInstrumentParamsChangedForTest
// pattern in internal/audio so cross-package tests can observe the audition
// without making real audio.
func SwapSynthAuditionFnForTest(fn func(string)) func(string) {
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
}

// auditionInstrumentAfterDrag fires one audition for an explicit user
// request. Its only production caller is previewActiveSynth (the Preview
// button) — knob releases and stage toggles are config-only and must not
// route here (see synthAuditionFn). instID is the already-resolved
// instrument id; the empty string is a no-op so callers don't have to gate
// on resolveSynthInstrument returning a value.
func (p *synthPanel) auditionInstrumentAfterDrag(instID string) {
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
func (p *synthPanel) resolveSynthInstrument(instID string) string {
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
func (p *synthPanel) synthTabActiveInstrument() string {
	dv := p.dv

	if dv.eqPanelZone != nil {
		ch := dv.eqPanelZone.ActiveChannel()
		if ch != "" && ch != "main" {
			return ch
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
	scrollBodyZ := ZEQPanel + 1
	z := ZEQPanel + 2
	// Capacity sized for the VISIBLE knob set (one expanded stage), not the
	// full schema: instEditorControls holds a slot per wired ParamDef (~1030 for
	// a modular recipe, almost all with empty rects), and reserving that many
	// HitAreas allocated ~100 KB per frame — a large slice of the Synth-tab
	// GC churn (see synth_live_param_edit_alloc_test.go). append grows the
	// slice in the rare frame where more controls are actually laid out.
	out := make([]HitArea, 0, 64)
	// Detail-pane scroll: a body catch-all (wheel + touch/drag scroll) and a
	// scrollbar thumb-drag handle when the selected stage's knobs overflow.
	// Collapsed sections (empty rect) publish nothing — guard explicitly,
	// because deriving the body from an empty rect would let image.Rect's
	// coordinate normalization fabricate a small hit area near the origin.
	for _, s := range dv.instEditorSections {
		if s.rect.Empty() {
			continue
		}
		grid := dv.sectionGrid(s.id)
		if !grid.HasScroll() {
			continue
		}
		body := image.Rect(
			s.rect.Min.X+synthSectionPaddingX,
			s.rect.Min.Y+dv.instEditorDetailHeaderH+synthSectionPaddingY,
			s.rect.Max.X-synthSectionPaddingX,
			s.rect.Max.Y-synthSectionPaddingY,
		)
		if body.Empty() {
			continue
		}
		out = append(out, HitArea{
			Rect:    body,
			ZIndex:  scrollBodyZ,
			Handler: &synthSectionScrollAdapter{dv: dv, id: s.id, body: true},
			Tag:     "synth-scrollbody-" + fmt.Sprint(int(s.id)),
		})
		thumb := grid.Scroll().BarRect()
		if thumb.Empty() {
			continue
		}
		out = append(out, HitArea{
			Rect:    thumb,
			ZIndex:  z + 1,
			Handler: &synthSectionScrollAdapter{dv: dv, id: s.id},
			Tag:     "synth-scroll-" + fmt.Sprint(int(s.id)),
		})
	}
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
	// Pipeline chips. z+1 so a chip wins over any underlying knob clip /
	// scroll catch-all (chips never overlap knobs geometrically, but the
	// touch-expanded ClipRects can reach).
	for i := range dv.instEditorChips {
		c := &dv.instEditorChips[i]
		if c.rect.Empty() {
			continue
		}
		out = append(out, HitArea{
			Rect:     c.rect,
			ZIndex:   z + 1,
			Handler:  &synthChipHitAdapter{dv: dv, id: c.id, instID: instID},
			Tag:      "synth-chip-" + sectionLabel(c.id),
			Touch:    true,
			ClipRect: clipFor(c.rect),
		})
	}
	// The selected stage's enable pill, in the detail-pane header. z+1 so
	// it wins over the pane's scroll catch-all.
	if sel := dv.synthSelectedSection(); sel != nil {
		if st := dv.synthStageEnableState(instID, sel); st.interactive {
			if pillR := dv.synthDetailEnablePillRect(); !pillR.Empty() {
				out = append(out, HitArea{
					Rect:     pillR,
					ZIndex:   z + 1,
					Handler:  &synthToggleHitAdapter{dv: dv, sec: sel.id, instID: instID},
					Tag:      "synth-toggle-" + st.param,
					Touch:    true,
					ClipRect: clipFor(pillR),
				})
			}
		}
	}
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
		case synthSaveButtonTag:
			tag = "synth-save"
		case synthSaveAsButtonTag:
			tag = "synth-save-as"
		case synthPreviewButtonTag:
			tag = "synth-preview"
		case synthResetButtonTag:
			tag = fmt.Sprintf("synth-btn-%d", i)
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
	instID string
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

// synthChipHitAdapter is the one-shot press handler for a pipeline chip: a
// tap only OPENS (selects) the stage so the next Layout expands it into the
// detail pane. It never enables/disables the stage — a ghost (disabled) chip
// opens to show its knobs (dimmed, behind the bypass scrim), and the user
// enables it explicitly via the detail-pane pill. Created fresh per Layout
// like every synth-tab adapter.
type synthChipHitAdapter struct {
	dv     *DrumView
	id     synthSectionID
	instID string
}

// glowRect implements glowTarget: hovering a pipeline chip lifts it through
// the shared tree hover overlay like every other clickable. The selected
// chip is suppressed — its lit active state is the signal there, mirroring
// latched buttons.
func (h *synthChipHitAdapter) glowRect(_, _ int, hitRect image.Rectangle) image.Rectangle {
	if h.dv != nil {
		if sel := h.dv.synthSelectedSection(); sel != nil && sel.id == h.id {
			return image.Rectangle{}
		}
	}
	return hitRect
}

func (h *synthChipHitAdapter) OnPress(x, y int) InputResult {
	if h.dv == nil {
		return InputIgnored
	}
	h.dv.setSelectedSynthSection(h.instID, h.id)
	if h.dv.eqPanelZone != nil {
		h.dv.eqPanelZone.Invalidate()
	}
	return InputCaptured
}

func (h *synthChipHitAdapter) OnDrag(x, y int)                     {}
func (h *synthChipHitAdapter) OnRelease(x, y int)                  {}
func (h *synthChipHitAdapter) OnWheel(x, y, steps int) InputResult { return InputIgnored }

// synthControlHitAdapter is the per-card hit handler: a tap/click opens the
// value-wheel popup (all platforms) and the mouse wheel nudges the value in
// place. The adapter does NOT cache the *Knob pointer — buildSynthTab can
// rebuild instEditorControls at any time (analyzer state changes, channel
// switches, …), which would leave a cached pointer dangling against a
// now-orphan widget. Looking up by index every call stays correct across
// re-layouts; each wheel notch reads instEditorControls[idx] and
// instEditorBindings[idx] fresh and mutually consistent. (The old rotary
// drag path — and the capture-by-index hazards it carried — was removed in
// the ParamCard conversion; the popup is now the only by-index editing
// surface, guarded by synthWheelPopup.IsOpen() at the schema-swap deferral.)
type synthControlHitAdapter struct {
	dv     *DrumView
	idx    int
	instID string
	arb    dragScrollArbiter // tap-vs-scroll gate for the current gesture
}

// ownerGrid returns the ControlGrid of the currently expanded stage — the grid
// that hosts this control and that a vertical sweep starting on it scrolls.
func (h *synthControlHitAdapter) ownerGrid() *ControlGrid {
	if h.dv == nil {
		return nil
	}
	if sel := h.dv.synthSelectedSection(); sel != nil {
		return h.dv.sectionGrid(sel.id)
	}
	return nil
}

func (h *synthControlHitAdapter) currentKnob() *Knob {
	if h.dv == nil || h.idx < 0 || h.idx >= len(h.dv.instEditorControls) {
		return nil
	}
	return h.dv.instEditorControls[h.idx]
}

// glowRect implements glowTarget: hovering a param card lifts it through the
// shared tree hover overlay like every other clickable. The whole card is the
// hover target (a tap anywhere on it opens the value-wheel popup).
func (h *synthControlHitAdapter) glowRect(_, _ int, hitRect image.Rectangle) image.Rectangle {
	return hitRect
}

func (h *synthControlHitAdapter) invalidatePanel() {
	if h.dv != nil && h.dv.eqPanelZone != nil {
		h.dv.eqPanelZone.Invalidate()
	}
}

func (h *synthControlHitAdapter) OnPress(x, y int) InputResult {
	// Numeric editor open: swallow the tap so nothing starts beneath it.
	if h.dv.paramEditor != nil && h.dv.paramEditor.Active() {
		return InputConsumed
	}
	// Capture the gesture and begin tap-vs-scroll arbitration: a vertical sweep
	// starting on this control scrolls its stage's ControlGrid, while a stationary
	// tap opens the value-wheel popup on RELEASE. Capturing (not consuming) is what
	// lets the sweep become a scroll instead of opening the popup on the down frame
	// — the whole point of the mobile "swipe up/down" fix. Desktop wheel-over-control
	// stays value-only (OnWheel), so rows are scrolled from the widened scrollbar.
	h.arb.begin(y, h.ownerGrid())
	return InputCaptured
}

// OnDrag advances the stage grid while the finger sweeps; once travel crosses the
// tap slop the gesture is a scroll and the release will NOT open the popup.
func (h *synthControlHitAdapter) OnDrag(x, y int) {
	if h.arb.drag(y) {
		h.invalidatePanel()
	}
}

// OnRelease opens the value-wheel popup only when the gesture stayed a tap (never
// crossed the slop). A committed scroll opens nothing. Selecting the control
// (focus graph) also happens here so a scroll leaves no stray selection.
func (h *synthControlHitAdapter) OnRelease(x, y int) {
	if !h.arb.end() {
		return
	}
	h.dv.setSynthSelectedControl(h.instID, h.idx)
	h.dv.openSynthControlWheelPopup(h.idx, h.instID)
}

// OnWheel nudges the value by one StepMul (endless) / one detent (discrete) per
// notch, holding Shift for one-rung-finer. The whole spin collapses into ONE
// debounced undo step (see wheelNudger). Section scrolling stays on grid
// whitespace (synthSectionScrollAdapter), so the knob's wheel is value-only.
func (h *synthControlHitAdapter) OnWheel(x, y, steps int) InputResult {
	// Numeric editor open: swallow the notch so a wheel spin can't mutate the
	// param (and arm a second commit) beneath the inline editor — mirrors OnPress.
	if h.dv.paramEditor != nil && h.dv.paramEditor.Active() {
		return InputConsumed
	}
	k := h.currentKnob()
	if k == nil || steps == 0 {
		return InputIgnored
	}
	var badge *KnobStepBadge
	if h.idx >= 0 && h.idx < len(h.dv.instEditorStepBadges) {
		badge = h.dv.instEditorStepBadges[h.idx]
	}
	fine := isKeyPressed(ebiten.KeyShiftLeft) || isKeyPressed(ebiten.KeyShiftRight)
	if !nudgeKnobByWheel(k, badge, steps, fine) {
		return InputIgnored
	}
	h.dv.setSynthSelectedControl(h.instID, h.idx)
	h.dv.syncSliderFromKnob(h.idx)
	h.dv.propagateSynthSliderValue(h.idx, h.instID)
	instID := h.instID
	dv := h.dv
	dv.synthWheelNudge.bump(func() {
		dv.commitInstrumentParams(instID)
		dv.requestSynthMirror(instID)
	})
	return InputConsumed
}

// synthSectionScrollAdapter routes scroll input for one overflowing section
// card to that section's ControlGrid. With body == true it is the card-body
// catch-all (a grab on empty card space); with body == false it is the
// scrollbar thumb handle. Both use the same row-by-row stepped drag (no
// continuous/momentum path), but in OPPOSITE directions: the body grab scrolls
// naturally (content follows the finger — ControlGrid.BeginContentDrag) while
// the thumb grab is direct (the thumb follows the finger — BeginDrag). A scroll
// change calls Invalidate so the next Layout re-derives the knob rects for the
// new window. Created fresh per Layout (like synthControlHitAdapter) so it never
// caches a stale grid pointer.
type synthSectionScrollAdapter struct {
	dv   *DrumView
	id   synthSectionID
	body bool
}

func (h *synthSectionScrollAdapter) grid() *ControlGrid {
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
	g := h.grid()
	if g == nil {
		return InputIgnored
	}
	// Both grabs start a row-by-row stepped drag anchored at the press point
	// (no continuous shift, no momentum fling), but in opposite directions: the
	// body grab follows the finger (natural), the thumb grab follows the thumb.
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
	g := h.grid()
	if g == nil {
		return
	}
	if g.DragTo(y) {
		h.invalidate()
	}
}

func (h *synthSectionScrollAdapter) OnRelease(x, y int) {
	if g := h.grid(); g != nil {
		g.EndDrag()
	}
}

func (h *synthSectionScrollAdapter) OnWheel(x, y, steps int) InputResult {
	g := h.grid()
	if g == nil {
		return InputIgnored
	}
	// One deliberate row per notch, throttled by the cooldown so a continuous
	// spin can't fly through the list. Always consume so the wheel never leaks
	// to a lower handler while the cursor is over the card.
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

// SynthTabChips returns the pipeline chip strip model (tests, scenes, JS
// bridge). One entry per stage, in audio-pipeline order.
func (p *synthPanel) SynthTabChips() []synthChip {
	dv := p.dv
	return dv.instEditorChips
}

// SelectSynthSectionByLabel opens the stage whose chip label matches
// (case-insensitive, e.g. "OSC", "ENVELOPE"). Routes through the same
// selection path as a chip tap (minus the ghost-enable side effect) so the
// JS bridge and scene catalog drive exactly what a user click drives.
// Returns false when no chip carries the label.
func (p *synthPanel) SelectSynthSectionByLabel(label string) bool {
	dv := p.dv

	instID := dv.synthTabActiveInstrument()
	if instID == "" {
		return false
	}
	for _, c := range dv.instEditorChips {
		if strings.EqualFold(sectionLabel(c.id), label) {
			dv.setSelectedSynthSection(instID, c.id)
			if dv.eqPanelZone != nil {
				dv.eqPanelZone.Invalidate()
			}
			return true
		}
	}
	return false
}

// SynthSectionGrid exposes a section's adaptive control grid for tests
// (column count, scroll state, visible window). Returns nil if the section has
// not been laid out yet.
func (p *synthPanel) SynthSectionGrid(id synthSectionID) *ControlGrid {
	dv := p.dv

	if dv.instEditorSectionGrids == nil {
		return nil
	}
	return dv.instEditorSectionGrids[id]
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
func (p *synthPanel) synthSaveDirty(instID string) bool {
	dv := p.dv
	_ = dv

	if instID == "" {
		return false
	}
	recipeID := audio.RecipeForInstrument(instID)
	if recipeID == "" {
		return false
	}
	return audio.InstrumentParamsDiffer(instID, recipeID)
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

func (dv *DrumView) toggleSynthStage(instID, paramName string) {
	dv.synp().toggleSynthStage(instID, paramName)
}

func (dv *DrumView) selectedSynthSection(instID string, sections []synthSection) synthSectionID {
	return dv.synp().selectedSynthSection(instID, sections)
}

func (dv *DrumView) setSelectedSynthSection(instID string, id synthSectionID) {
	dv.synp().setSelectedSynthSection(instID, id)
}

func (dv *DrumView) synthSelectedSection() *synthSection {
	return dv.synp().synthSelectedSection()
}

func (dv *DrumView) synthStageEssential(instID, enableParam string) bool {
	return dv.synp().synthStageEssential(instID, enableParam)
}

func (dv *DrumView) synthDetailEnablePillRect() image.Rectangle {
	return dv.synp().synthDetailEnablePillRect()
}

func (dv *DrumView) synthStageEnableState(instID string, sec *synthSection) synthEnableState {
	return dv.synp().synthStageEnableState(instID, sec)
}

func (dv *DrumView) applySynthStageToggleByID(instID string, id synthSectionID) {
	dv.synp().applySynthStageToggleByID(instID, id)
}

func (dv *DrumView) buildSynthTab(contentR image.Rectangle, instID string) {
	dv.synp().buildSynthTab(contentR, instID)
}

func (dv *DrumView) layoutSynthHeader(contentR image.Rectangle, headerH int, instID, recipeID string, mobile bool) {
	dv.synp().layoutSynthHeader(contentR, headerH, instID, recipeID, mobile)
}

func (dv *DrumView) buildSynthHeaderButtons(resolved string) {
	dv.synp().buildSynthHeaderButtons(resolved)
}

func (dv *DrumView) layoutSynthSections(rowR image.Rectangle, sections []synthSection, mobile bool, instID string) {
	dv.synp().layoutSynthSections(rowR, sections, mobile, instID)
}

func (dv *DrumView) appendSynthChip(s synthSection, r image.Rectangle, instID string, selID synthSectionID) {
	dv.synp().appendSynthChip(s, r, instID, selID)
}

func (dv *DrumView) sectionGrid(id synthSectionID) *ControlGrid {
	return dv.synp().sectionGrid(id)
}

func (dv *DrumView) synthTabUpdate() bool {
	return dv.synp().synthTabUpdate()
}

func (dv *DrumView) synthSectionTierSplit(knobIdxs []int) (essential, advanced []int) {
	return dv.synp().synthSectionTierSplit(knobIdxs)
}

func (dv *DrumView) placeKnobsInSection(s synthSection, mobile bool) {
	dv.synp().placeKnobsInSection(s, mobile)
}

func (dv *DrumView) drawSynthTab(dst *ebiten.Image, contentR image.Rectangle, instID string) {
	dv.synp().drawSynthTab(dst, contentR, instID)
}

func (dv *DrumView) synthDecayValueForPreview() float64 {
	return dv.synp().synthDecayValueForPreview()
}

func (dv *DrumView) drawSynthOutRow(dst *ebiten.Image, paneR image.Rectangle, instID string) {
	dv.synp().drawSynthOutRow(dst, paneR, instID)
}

func (dv *DrumView) drawSynthHeader(dst *ebiten.Image) {
	dv.synp().drawSynthHeader(dst)
}

func (dv *DrumView) drawSynthChipStrip(dst *ebiten.Image) {
	dv.synp().drawSynthChipStrip(dst)
}

func (dv *DrumView) instEditorchipPrev(i int) synthChip {
	return dv.synp().instEditorchipPrev(i)
}

func (dv *DrumView) drawSynthDetailPane(dst *ebiten.Image, instID string) {
	dv.synp().drawSynthDetailPane(dst, instID)
}

func (dv *DrumView) drawSynthFooter(dst *ebiten.Image) {
	dv.synp().drawSynthFooter(dst)
}

func (dv *DrumView) handleSynthTabInput(x, y int, pressed bool, instID string) bool {
	return dv.synp().handleSynthTabInput(x, y, pressed, instID)
}

func (dv *DrumView) syncSliderFromKnob(idx int) {
	dv.synp().syncSliderFromKnob(idx)
}

func (dv *DrumView) synthControlReadoutRect(idx int) image.Rectangle {
	return dv.synp().synthControlReadoutRect(idx)
}

func (dv *DrumView) setKnobReadoutRect(idx int, r image.Rectangle) {
	dv.synp().setKnobReadoutRect(idx, r)
}

func (dv *DrumView) openSynthParamEditor(idx int, instID string) {
	dv.synp().openSynthParamEditor(idx, instID)
}

func (dv *DrumView) propagateSynthSliderValue(idx int, instID string) {
	dv.synp().propagateSynthSliderValue(idx, instID)
}

func (dv *DrumView) previewActiveSynth() {
	dv.synp().previewActiveSynth()
}

func (dv *DrumView) auditionInstrumentAfterDrag(instID string) {
	dv.synp().auditionInstrumentAfterDrag(instID)
}

func (dv *DrumView) resolveSynthInstrument(instID string) string {
	return dv.synp().resolveSynthInstrument(instID)
}

func (dv *DrumView) synthTabActiveInstrument() string {
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

func (dv *DrumView) SynthTabChips() []synthChip {
	return dv.synp().SynthTabChips()
}

func (dv *DrumView) SelectSynthSectionByLabel(label string) bool {
	return dv.synp().SelectSynthSectionByLabel(label)
}

func (dv *DrumView) SynthSectionGrid(id synthSectionID) *ControlGrid {
	return dv.synp().SynthSectionGrid(id)
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

func (dv *DrumView) synthSaveDirty(instID string) bool {
	return dv.synp().synthSaveDirty(instID)
}

func (dv *DrumView) SynthTabHeader() synthHeaderLayout {
	return dv.synp().SynthTabHeader()
}

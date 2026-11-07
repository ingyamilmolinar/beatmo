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

// The Synth tab: one pipeline surface in the same order for every synth instrument, a disabled stage dimmed
// but interactive. A tab rather than a portal because the audio panel is where users already inspect a signal chain.

// ---- Section model -----------------------------------------------------

type synthSectionID int

const (
	synthSectionPitch synthSectionID = iota
	synthSectionEnvelope
	synthSectionTone
	synthSectionDrive
	// synthSectionDrive is the canonical POST section; it keeps its name, labelled "POST".
	synthSectionOsc
	synthSectionFM
	synthSectionFilter
	// VOICE holds the knobs that ARE the sound (the family voice, the generator picker); it is pruned when empty,
	// and when shown its pill is locked ON because the voice cannot be disabled.
	synthSectionVoice
	// These modulators ship DISABLED everywhere (off = the render without them), with audible values behind the pill.
	synthSectionLFO
	synthSectionBurst
	// Its own section so FILTER's pill stays filter_enabled and the envelope gets a real pill of its own.
	synthSectionFilterEnv
	// Only for modular kick instruments; legacy drum-kick recipes keep their kick knobs in VOICE.
	synthSectionKick
	// ENSEMBLE and RESONATOR are value-gated (voices==1, body_model Off) rather than *_enabled, so their pills
	// drive that value; FORMANT has a real pill.
	synthSectionEnsemble
	synthSectionFormant
	synthSectionResonator
	// Always on (each op is a no-op at its identity), so no pill.
	synthSectionFinish
	// The WAV-clone stages, present only on documents that declare them; appended so no id is renumbered.
	synthSectionModal
	synthSectionStrike
	synthSectionNoise
)

const synthSectionPost = synthSectionDrive

// The same sections in the same order for every synth sound. Empty sections are pruned (VOICE without family
// knobs, FM where nothing routes to it); FINISH has no pill.
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

// A section survives only with a knob or a pill; POST always does (every recipe carries gain).
func synthSectionOrderForSchema(schema []audio.ParamDef) []synthSectionID {
	vals := schemaValues(schema)
	hasContent := map[synthSectionID]bool{}
	for _, d := range schema {
		if d.Group == audio.SynthHiddenGroup || !audio.FlatFieldDeclared(vals, d.Name) {
			continue
		}
		// Collision-excluded stage params are absent from the schema, so they cannot conjure an empty card.
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

// "" when the full label is already compact.
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

// Full label, else the designed short one, else an ellipsis: a mid-word "ENVELOP…" reads worse than "ENV".
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

// Separate from the canonical English sectionLabel, which backs hit-area tags and the JS lookup and must not
// shift with the language.
func sectionLabelLocalized(id synthSectionID) string {
	k := sectionLabelKey(id)
	if k == "" {
		return sectionLabel(id)
	}
	return i18n.T(k)
}

// "" for an unknown section.
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

// Shown beside the jargon title, so the card teaches the vocabulary without hiding the professional term.
func sectionSubtitle(id synthSectionID) string {
	k := sectionSubtitleKey(id)
	if k == "" {
		return ""
	}
	return i18n.T(k)
}

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
	// The kick is a family voice like snare/cym, so its toggle lives with its knobs.
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

// Only for genuine appended stage params; a same-named family knob routes by the family rules.
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
	// No pill: voices==1 is the off-state.
	case "unison":
		return synthSectionEnsemble, true
	// Phase-15 FORMANT vowel-bank stage (Task 8).
	case "formant":
		return synthSectionFormant, true
	// No pill: body_model Off is the off-state.
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
	// PITCH ENV reuses the PITCH section id.
	case "pitchenv":
		return synthSectionPitch, true
	case "lfo":
		return synthSectionLFO, true
	case "burst":
		return synthSectionBurst, true
	}
	return 0, false
}

// Identified by the seeded gen1_source, never a knob name: a config-first instrument's knobs are
// gen1_<family>_*, so a name check misclassifies it.
func activeFamilyStage(schema []audio.ParamDef) (audio.ModularFamilyStage, bool) {
	for _, d := range schema {
		if d.Name == "gen1_source" {
			return audio.FamilyStageForSource(d.Default)
		}
	}
	return audio.ModularFamilyStage{}, false
}

// Each Default is filled from the instrument's document.
func schemaValues(schema []audio.ParamDef) map[string]float64 {
	out := make(map[string]float64, len(schema))
	for _, d := range schema {
		out[d.Name] = d.Default
	}
	return out
}

// The canonical router: stage params by their stage Group, the generic post knobs to POST (decay to
// ENVELOPE), and everything else to VOICE, because the family knobs ARE the instrument's sound.
func sectionForParamDef(def audio.ParamDef) synthSectionID {
	name := def.Name
	// By the toggle's group whatever its appended status: drive_enabled is a real POST toggle though DRIVE's
	// knob is collision-excluded everywhere.
	if isSynthEnableParam(name) {
		if sec, ok := enableToggleSection(name); ok {
			return sec
		}
	}
	// Gen-bank family knobs are the voice's own, so VOICE. Matched by NAME: the group "fm" is shared with
	// the modular FM STAGE, whose knobs must keep routing to synthSectionFM below.
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
	return synthSectionVoice
}

func sectionForParam(name string) synthSectionID {
	if def, ok := audio.StageParamDefByName(name); ok {
		return sectionForParamDef(def)
	}
	return sectionForParamDef(audio.ParamDef{Name: name})
}

// Rendered as a pill, never a knob.
func isSynthEnableParam(name string) bool {
	return strings.HasSuffix(name, "_enabled")
}

// The SINGLE source of "no UI on this recipe in its current state", the grid's own predicate, so other callers
// cannot drift from what renders. family/familyOK only keep the callers' signature.
func synthParamHiddenInContext(def audio.ParamDef, family audio.ModularFamilyStage, familyOK bool, merged audio.RecipeParams) bool {
	return def.Group == audio.SynthHiddenGroup ||
		!audio.FlatFieldDeclared(merged, def.Name) ||
		synthCymbalPlateParamHidden(def, merged) ||
		synthVoiceFreqParamHidden(def, merged)
}

// The one value-conditional rule module types cannot express: hat (1) and crash (5) run the plate only once
// Plate Modes ≥ 1.
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

// Gated on the recipe default, never the live value, so the knob cannot vanish mid-edit.
func synthVoiceFreqParamHidden(def audio.ParamDef, _ audio.RecipeParams) bool {
	return def.Name == "voice_freq_hz" && def.Default <= 0
}

// The form any per-recipe assertion must use: a param hidden in context has no knob to select.
func synthParamIsGridControlIn(def audio.ParamDef, family audio.ModularFamilyStage, familyOK bool, merged audio.RecipeParams) bool {
	return !synthParamHiddenInContext(def, family, familyOK, merged) && !isSynthEnableParam(def.Name)
}

// Context-free: whether a param can EVER be a knob, not whether this recipe shows it.
func synthParamIsGridControl(def audio.ParamDef) bool {
	return def.Group != audio.SynthHiddenGroup && !isSynthEnableParam(def.Name)
}

// Missing = enabled (the shipped default), so it is safe for non-modular instruments.
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

// A sole sound source (OSC on a pure-oscillator voice) gets a greyed, inert pill, so the one thing making the
// sound cannot be switched off.
func (p *synthPanel) synthStageEssential(instID instrument.ID, enableParam string) bool {
	if enableParam == "" {
		return false
	}
	return audio.StageDisableSilences(p.dv.resolveSynthInstrument(instID), enableParam)
}

// Config-only: the NEXT trigger renders with the stage flipped. No audition - a full-gain one-shot over the
// sequencer's voices was heard as a crackle; Preview is the only audition.
func (p *synthPanel) toggleSynthStage(instID instrument.ID, paramName string) {
	dv := p.dv

	instID = dv.resolveSynthInstrument(instID)
	if instID == "" || paramName == "" {
		return
	}
	// Backstop so no input path (tap, tree, scene, JS) can silence the instrument through its sole source.
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
	enableNone synthEnableKind = iota
	// Interactive unless it gates a sole sound source.
	enableBool
	// Off is a param VALUE (unison_voices 1, body_model Off), which the toggle drives.
	enableGate
	// VOICE: the voice is the instrument, so it is always on.
	enableLocked
)

// The single model the pill rect, draw, input and hit area consult.
type synthEnableState struct {
	kind        synthEnableKind
	enabled     bool
	interactive bool
	param       string
	onVal       float64
	offVal      float64
}

// Structural only; an empty VOICE is pruned before layout.
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
		// FINISH is always on too: each op is a no-op at its identity.
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
		return p.synthGateEnableState(instID, "unison_voices", 2, 1, 3)
	case sec.id == synthSectionResonator:
		return p.synthGateEnableState(instID, "body_model", 1, 0, 1)
	}
	return synthEnableState{kind: enableNone}
}

// The ON value is the library value when that is active itself, else onFallback.
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

// The single write path behind the pill; a sole-source generator is refused even if reached.
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

// By id: sections are rebuilt each layout, so a pointer would dangle.
func (p *synthPanel) applySynthStageToggleByID(instID instrument.ID, id synthSectionID) {
	dv := p.dv
	for i := range dv.instEditorSections {
		if dv.instEditorSections[i].id == id {
			p.applySynthStageToggle(instID, &dv.instEditorSections[i])
			return
		}
	}
}

type synthSection struct {
	id       synthSectionID
	rect     image.Rectangle
	knobIdxs []int // indices into instEditorControls / instEditorBindings
	// Rendered as a pill, never a knob; "" when the recipe has no per-stage toggle here.
	enableParam string
}

// The one way a stage opens: the pipeline pill, the JS export, scenes and tests all route here.
func (p *synthPanel) setSelectedSynthSection(instID instrument.ID, id synthSectionID) {
	dv := p.dv
	instID = dv.resolveSynthInstrument(instID)
	if instID == "" {
		return
	}
	dv.setSynthStageOpen(instID, id)
}

// nil at the root or when the target spans several stages.
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

func (p *synthPanel) synthDetailEnablePillRect() image.Rectangle {
	return p.dv.instEditorMacroPillRect
}

// synthMacroCacheKey gates the per-Layout rebuild of the category tables.
type synthMacroCacheKey struct {
	inst      instrument.ID
	n         int
	hash      string
	voiceSecs float64 // the dials' "settles" gate depends on the note length
	// The cards rebuild once when the DSP's relevance verdict lands.
	relSig   uint64
	relKnown bool
}

type synthHeaderLayout struct {
	rect        image.Rectangle
	previewRect image.Rectangle
	saveAsRect  image.Rectangle
	// The same action menu the mobile sticky bar opens.
	moreRect  image.Rectangle
	instLabel instrument.ID // the instrument id (the header renders its display name)
}

type instParamBinding struct {
	def audio.ParamDef
}

// Sentinel texts: the slice is positional and routed by text; the leading \x00 keeps them off screen.
const (
	synthSaveAsButtonTag  = "\x00synth-save-as"
	synthPreviewButtonTag = "\x00synth-preview"
)

// Fixed: they encode the panel's visual rhythm, not control sizing.
const (
	synthSectionPaddingX = 8
	synthSectionPaddingY = 6
)

// The single synth-vs-WAV classifier; must agree with buildSynthTab's len(schema) == 0 banner check.
func instrumentHasSynthControls(instID instrument.ID) bool {
	if instID == "" {
		return false
	}
	// A synth instrument is one with an instrument document.
	return audio.HasInstrumentDoc(instID)
}

func (dv *DrumView) activeInstrumentHasSynth() bool {
	return instrumentHasSynthControls(dv.synthTabActiveInstrument())
}

// Mirrors the desktop pill rule so the mobile bottom-nav segments agree.
func (dv *DrumView) masterChannelSelected() bool {
	return dv.eqPanelZone != nil && dv.eqPanelZone.masterChannelSelected()
}

func (dv *DrumView) synthViewDisabled() bool {
	return dv.masterChannelSelected() || !dv.activeInstrumentHasSynth()
}

func (dv *DrumView) samplerViewDisabled() bool {
	return dv.masterChannelSelected()
}

// Knob instances are KEPT across Layouts while the binding sequence is unchanged: Layout runs every frame,
// and recreating them would reset an in-flight drag, so any per-frame reallocation here is a correctness bug.
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
	// The armed map is per instrument but the editor is shared: switching instruments must disarm or rebind, or
	// the schema-defer guard below freezes this tab on the old schema.
	if aidx, armed := dv.synthControlArmed(resolved); !armed {
		dv.instEditorArmedEditor = nil
	} else if e := dv.instEditorArmedEditor; e == nil || e.instID != resolved || e.idx != aidx {
		dv.instEditorArmedEditor = newSynthArmedControlEditor(dv, resolved, aidx)
	}
	// Derived from the instrument's own document: Default = its value, ranges widened to contain it.
	schema := audio.InstrumentParamDefs(resolved)

	// No editable params means a WAV sample: one banner instead of a grid of empty stage cards.
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

	// Read-only cached merge: a fresh map here was a ~115 KB allocation per frame.
	genMerged := audio.MergedInstrumentParamsRO(resolved)
	deferred := false
	// A schema swap while an armed editor is live would rebind its index to a DIFFERENT param (in WASM it drove
	// gain/osc_enabled and silenced the voice), so the knob set holds until it disarms.
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
		// Mobile: identity lives in the channel pill and the actions on the sticky bar, so no header.
		headerH = 0
	} else {
		headerH = Profile().SynthHeaderH
		// In a short panel the header shrinks so the sections keep minSectionsH.
		const minSectionsH = 60
		if contentR.Dy() < headerH+minSectionsH {
			headerH = contentR.Dy() - minSectionsH
		}
		// Never below one action-button row: the action buttons must always exist.
		if floor := Profile().DensityValues().SynthHeaderButtonH + 2*SpaceXS; headerH < floor {
			headerH = floor
		}
		if headerH > contentR.Dy() {
			headerH = contentR.Dy()
		}
	}

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
	// Desktop only; mobile keeps a full-width stack so knobs stay touch-friendly.
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
		dv.instEditorMobileFocusRect = image.Rectangle{}
	} else {
		// Narrow desktop panel with no preview pane: no mobile band either.
		dv.instEditorPreviewRect = image.Rectangle{}
		dv.instEditorMobileFocusRect = image.Rectangle{}
	}

	// Instances are reused when len(schema) and every param name match, so a drag survives the re-layout.
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
			// A widget slot is still allocated so knob and binding indices stay aligned.
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
			// A re-voiced bespoke instrument lacks the modular keys; the ParamDef default matches the C dispatch.
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
		// Rendering polarity only, so it is safe to set mid-drag.
		bipolar := def.Min < 0 && def.Max > 0
		zeroFrac := 0.5
		if bipolar && def.Max > def.Min {
			zeroFrac = (0 - def.Min) / (def.Max - def.Min)
		}
		sc, discrete, endless := scaleFromParamDef(def)
		if reuseExisting && i < len(dv.instEditorControls) {
			// Mid-drag the finger owns the value; refresh only when none is in progress.
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

	// Prune stages with no visible knob and no pill: the modular VOICE's physical-model knobs route here
	// but are hidden for the current osc_type, so the candidate order kept an empty VOICE.
	kept := sections[:0]
	for _, s := range sections {
		if len(s.knobIdxs) == 0 && s.enableParam == "" {
			continue
		}
		kept = append(kept, s)
	}
	sections = kept
	// Stored BEFORE the category layout, which reads it.
	dv.instEditorSections = sections

	// Rebuilt only when the instrument, schema or params change: buildMacroGroups walks the whole schema.
	voiceSecs := dv.synthVoiceSecs(resolved)
	macroKey := synthMacroCacheKey{inst: resolved, n: len(schema), hash: p.synthParamsHash(resolved), voiceSecs: voiceSecs}
	macros := dv.instEditorMacros
	// A dial drag keeps the dial set: a turn may flip another dial's liveness (Length gates Hold), and the row
	// must not reshuffle under the finger.
	dv.healSynthDialGesture()
	frozen := dv.synthDialActive != nil && dv.synthDialActive.inst == resolved && macros != nil &&
		dv.instEditorMacrosKey.inst == resolved
	if !frozen {
		// Also once at an instrument's first sight, to list the dials the DSP measures.
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
	// Unreachable for a non-empty schema; a defensive no-op layout.
	dv.instEditorSectionsRect = sectionsRect
	dv.buildSynthHeaderButtons(resolved)
	dv.instEditorScrollMax = 0
	dv.instEditorScrollPx = 0
}

// So a deferred Layout keeps the exact knob set live through a drag.
func schemaFromBindings(bindings []instParamBinding) []audio.ParamDef {
	out := make([]audio.ParamDef, len(bindings))
	for i, b := range bindings {
		out[i] = b.def
	}
	return out
}

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

// Laid out exactly like the Sampler header so the tabs read as one family; mobile lays out nothing in-panel.
func (p *synthPanel) layoutSynthHeader(contentR image.Rectangle, headerH int, instID instrument.ID, mobile bool) {
	dv := p.dv

	if mobile {
		dv.instEditorHeader = synthHeaderLayout{}
		dv.instEditorHeaderBtns = dv.instEditorHeaderBtns[:0]
		return
	}
	if headerH <= 0 {
		// Too short for a band (test sizes); the identity is still recorded for its readers.
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

// Built once; the sticky bar owns their per-frame layout.
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

// nil off-mobile and for an instrument with no synth controls.
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

// Save As goes Primary when dirty so the signal stays honest; an empty rect skips its button.
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

// One grid serves every drilled target; the id stays for callers that address grids by stage.
func (p *synthPanel) sectionGrid(_ synthSectionID) *ControlGrid {
	dv := p.dv
	if dv.instEditorMacroGrid == nil {
		dv.instEditorMacroGrid = NewControlGrid(controlGridScrollbarStyle())
		// Continuous pixel + momentum content scroll (the stage grids' feel).
		dv.instEditorMacroGrid.SetSmoothScroll(true)
	}
	return dv.instEditorMacroGrid
}

func (p *synthPanel) SynthSectionGrid(id synthSectionID) *ControlGrid {
	return p.sectionGrid(id)
}

// The name the armed-control and card-sprite tests drive the card loop through.
func (p *synthPanel) drawSynthDetailPane(dst *ebiten.Image, instID instrument.ID) {
	p.drawSynthMacroLayer(dst, p.dv.resolveSynthInstrument(instID))
}

// Returns true while a fling coasts, so the knob rects follow the scroll window.
func (p *synthPanel) synthTabUpdate() bool {
	dv := p.dv

	dv.synthWheelNudge.tick()

	changed := false
	if g := dv.instEditorMacroGrid; g != nil {
		g.Tick()
		if g.UpdateMomentum() {
			changed = true
		}
	}
	// Arrows are NOT read here: the armed control is a partial claimant, and reading them twice steps twice per
	// press (non-consuming). Esc is not handled here either: handleEscape is the single Esc authority.
	if instID := dv.synthTabActiveInstrument(); instID != "" {
		if _, armed := dv.synthControlArmed(instID); armed && dv.instEditorArmedEditor != nil {
			if dv.instEditorArmedEditor.Tick() {
				changed = true
			}
		}
	}
	// Runs after the lock too (the shrink); a live animation forces a relayout each frame.
	if dv.advanceArmGrow() {
		changed = true
	}
	// The trigger glow animates until the note fades.
	if dv.glow != nil && dv.glow.tick() {
		changed = true
	}
	// Keep drawing while a render is pending, so the "updating" chip and the landing show.
	if dv.graphPipe != nil && (dv.graphPipe.dirty.Load() || dv.graphPipe.pending(slotNow)) {
		changed = true
	}
	return changed
}

// Generous on purpose: the ParamCard ideal width limits the real columns; the cap bounds pathological wide
// panes and the divider layout's column probe.
const synthMaxGridCols = 12

func synthControlCellHeight() int {
	// Card-cell layout (Task 3): each grid cell is a full ParamCard slot.
	return Profile().DensityValues().ParamCardH
}

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

// So the More band never rides into the grown armed card.
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

// Below the More divider: neighbours the card covers are hidden for the duration.
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

// The caller's uniform layout IS the resting layout; every other control slides to a free cell, never hidden.
// The block widens past 2 columns only when 2 cells are too narrow for the two lanes.
func (dv *DrumView) growArmedCard(visible []int, grid *ControlGrid, innerR image.Rectangle, armedKIdx int) {
	prog := dv.armGrowProgress()
	if prog <= 0 || grid == nil {
		return
	}
	cols := grid.Cols()
	if cols < 1 {
		return
	}
	// Divider sections keep the instant grow: the reflow's full-width band needs more rows than a short pane has.
	for i, k := range visible {
		if k < 0 {
			// An armed control in the essentials prefix reflows just that prefix; one below the divider grows in place.
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
	// Phantom (divider) cells read their grid slot.
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
	// A neighbour pushed below the window gets an empty target, a zero rect at the SCREEN ORIGIN: lerping toward
	// it flew the control up into nothing. Re-run the reflow unclipped so it travels down; the clip cuts it at the fold.
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
	// Reach SynthArmedCardH so the lanes have vertical room even when the block's lower cells are off-window.
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
			// Already off-window at rest: lerping from the empty sentinel would fly it out of the top-left corner, and the
			// reflow only pushes down, so it stays hidden.
			r = image.Rectangle{}
		default:
			r = lerpRect(resting[i], target[i], prog)
			// Cut at the fold so it slides under the grid's bottom edge instead of painting over what follows.
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

	if dv.instEditorNoSynth {
		drawSynthNoSynthBanner(dst, dv.instEditorBannerRect)
		return
	}

	dv.drawSynthHeader(dst)
	p.drawSynthMacroLayer(dst, resolved)
	// The graph is drawn from the rendered note: the overview at the root, the focused control's view otherwise.
	if r := dv.instEditorPreviewRect; !r.Empty() {
		p.drawSynthGraphPane(dst, r, resolved)
	}
	// Mobile has no side pane: the graph is a band above the controls.
	if mb := dv.instEditorMobileFocusRect; !mb.Empty() {
		p.drawSynthGraphPane(dst, mb, resolved)
	}
	dv.drawSynthFooter(dst)
	dv.drawSaveAsDialog(dst)
}

// The action buttons are drawn by drawSynthFooter.
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

// The shared renderer, so the Synth and Sampler tabs draw an identical armed card.
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

// Package-level so tests assert the exact text.
const (
	synthNoSynthHeadline = "This instrument does not use the synth"
	synthNoSynthSubtitle = "It plays a WAV sample — open the Sampler tab to edit it."
)

func drawSynthNoSynthBanner(dst *ebiten.Image, contentR image.Rectangle) {
	if contentR.Empty() {
		return
	}
	// An inset card so the banner reads as one deliberate surface.
	cardR := contentR.Inset(SpaceMD)
	if cardR.Empty() {
		cardR = contentR
	}
	drawRoundedRect(dst, cardR, TokenSurface1(), RadiusSM, true)
	drawRoundedRect(dst, cardR, TokenBorderSubtle(), RadiusSM, false)

	cx := cardR.Min.X + cardR.Dx()/2
	availW := cardR.Dx() - 2*SpaceMD

	// Rune-safe clipping so the hint is never dropped on a narrow panel.
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

// Sentinel texts become labels at draw time, so the \x00 markers never reach the screen.
func (p *synthPanel) drawSynthFooter(dst *ebiten.Image) {
	dv := p.dv

	for _, btn := range dv.instEditorBtns {
		if btn == nil || btn.Text == synthMoreButtonTag {
			// The More divider draws its own chrome; its Button is input-only.
			continue
		}
		// Without mutating Text, which still routes hits and test lookups.
		savedText := btn.Text
		if label := synthHeaderButtonLabel(btn.Text); label != "" {
			btn.Text = label
		}
		btn.Draw(dst)
		btn.Text = savedText
	}
}

// "" for a non-sentinel text.
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

// The single source of wording, so width measurement and draw agree; a longer translation is abbreviated here.
func synthActionLabel(action string) string {
	switch action {
	case "preview":
		return i18n.T(i18n.KeyPreview)
	case "save-as":
		return i18n.T(i18n.KeySaveAs)
	}
	return ""
}

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
	// The name lives in the stage title; for an enum the label IS the value.
	if lbl := enumLabelFor(def, value); lbl != "" {
		return lbl
	}
	return synthParamDisplayName(def.Name) + "  " + formatSynthParamValue(def, value)
}

// "" when nothing maps; the caller skips the line.
func synthControlPurpose(def audio.ParamDef) string {
	if p := PlainEnglish(def.Label); p != "" {
		return p
	}
	return PlainEnglish(def.Name)
}

// The section title carries the name, so a narrow cell keeps the value legible.
func synthControlCaptionValueOnly(def audio.ParamDef, value float64) string {
	if lbl := enumLabelFor(def, value); lbl != "" {
		return lbl
	}
	return formatSynthParamValue(def, value)
}

// Name only: the pill above shows the value, and both printed it twice.
func synthControlMobileCaption(def audio.ParamDef) string {
	return synthParamDisplayName(def.Name)
}

// Enum or Step>0 is discrete; otherwise endless.
func scaleFromParamDef(def audio.ParamDef) (sc KnobScale, discrete, endless bool) {
	sc = KnobScale{Min: def.Min, Max: def.Max, Unit: def.Unit, Enum: def.Enum, Step: def.Step}
	discrete = len(def.Enum) > 0 || def.Step > 0
	endless = !discrete
	return
}

func (dv *DrumView) knobStepPref(name string) (float64, bool) {
	sink := activeKnobStepSink()
	if sink == nil {
		return 0, false
	}
	steps := sink.LoadKnobSteps()
	v, ok := steps[name]
	return v, ok
}

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

func (p *synthPanel) handleSynthTabInput(x, y int, pressed bool, instID instrument.ID) bool {
	dv := p.dv

	instID = dv.resolveSynthInstrument(instID)
	if !pressed {
		return false
	}
	// A direct path kept for tests and the JS bridge; production input is the tree-routed card tap. The stage
	// pill is hit-tested first so a header tap toggles it instead of falling through.
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

// The propagate/coalesce path reads Slider.Value.
func (p *synthPanel) syncSliderFromKnob(idx int) {
	dv := p.dv

	if idx < 0 || idx >= len(dv.instEditorControls) || idx >= len(dv.instEditorSliders) {
		return
	}
	dv.instEditorSliders[idx].Value = dv.instEditorControls[idx].Value
}

// ---- Readout-rect helpers ----------------------------------------------

// ---- Hit areas + propagate ---------------------------------------------

// Coalesced so a still drag does not flood the JS bridge.
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
	// An exact selector index: the C engine reads it via lrintf, and JSON round-trips without fractional drift.
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

// Only the explicit Preview button calls this, through audio.Play's own trigger path. Knob releases and
// stage toggles must NOT: a full-gain one-shot over the sequencer's voices overshot the master and crackled.
var synthAuditionFn = func(instID instrument.ID) {
	audio.Play(instID)
}

func SwapSynthAuditionFnForTest(fn func(instrument.ID)) func(instrument.ID) {
	prev := synthAuditionFn
	if fn != nil {
		synthAuditionFn = fn
	}
	return prev
}

// Knob edits already wrote through, so a plain trigger plays the current unsaved tone.
func (p *synthPanel) previewActiveSynth() {
	dv := p.dv

	dv.auditionInstrumentAfterDrag(dv.synthTabActiveInstrument())
	// The glow follows the audition through the same sync sink as a sequencer hit.
	dv.onInstrumentTriggered(dv.resolveSynthInstrument(dv.synthTabActiveInstrument()))
}

// Only the Preview button may call this. An empty id is a no-op.
func (p *synthPanel) auditionInstrumentAfterDrag(instID instrument.ID) {
	dv := p.dv
	_ = dv

	if instID == "" {
		return
	}
	synthAuditionFn(instID)
}

func (p *synthPanel) resolveSynthInstrument(instID instrument.ID) instrument.ID {
	dv := p.dv

	if instID != "" {
		return instID
	}
	return dv.synthTabActiveInstrument()
}

// The EQ panel's active channel, else the first non-empty row, as the other audio tabs resolve it.
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

func (p *synthPanel) synthTabHitAreas() []HitArea {
	dv := p.dv

	instID := dv.synthTabActiveInstrument()
	// Knobs sit above the per-section scroll catch-all, so a knob press captures the knob and an empty-space
	// press scrolls; pills, dialog and footer sit higher still.
	z := ZEQPanel + 2
	// Sized for the VISIBLE set: the full schema's ~1000 slots meant ~100 KB of HitAreas per frame.
	out := make([]HitArea, 0, 64)
	// The touch-min clip is centred on the visual rect, so it forgives near misses without moving the press point.
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
		// Also under the "synth-slider-%d" tag, which existing tests target.
		out = append(out, HitArea{
			Rect:     r,
			ZIndex:   z,
			Handler:  &synthControlHitAdapter{dv: dv, idx: i, instID: instID},
			Tag:      fmt.Sprintf("synth-slider-%d", i),
			Touch:    true,
			ClipRect: clip,
		})
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
	// The drilled knobs are published above; Back and More are in instEditorBtns.
	out = append(out, p.synthMacroHitAreas(instID)...)
	// Above the footer buttons.
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
		// Consumes clicks inside the dialog so they never read as a click outside.
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
		// Each button dispatches through its own OnClick, never by text; the tag keeps per-button routing tests working.
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
		// One tier above knobs, so a click on a footer rect never triggers the control beneath.
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

// Created fresh per Layout so it never caches a stale pointer.
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

// Not armed: a still tap arms, a sweep or wheel scrolls the grid. Armed: input spins the lane under the
// pointer and a still tap locks. Looks the knob up by index every call: buildSynthTab may rebuild the slice.
type synthControlHitAdapter struct {
	dv     *DrumView
	idx    int
	instID instrument.ID
	arb    dragScrollArbiter // tap-vs-scroll gate for the current gesture (NOT-armed path only)

	// Cumulative displacement past the tap slop, not "a whole detent crossed": a 12–20 px drag crosses no
	// 22–32 px detent and would otherwise lock instead of spinning.
	armedMoved  bool
	armedStartX int
	armedStartY int
}

func (h *synthControlHitAdapter) isArmed() bool {
	idx, armed := h.dv.synthControlArmed(h.instID)
	return armed && idx == h.idx
}

func (h *synthControlHitAdapter) ownerGrid() *ControlGrid {
	if h.dv == nil {
		return nil
	}
	return h.dv.instEditorMacroGrid
}

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
		// A still press+release locks (OnRelease).
		h.armedMoved = false
		h.armedStartX = x
		h.armedStartY = y
		if h.dv.instEditorArmedEditor != nil {
			h.dv.instEditorArmedEditor.BeginDrag(x, y)
		}
		return InputCaptured
	}
	// Capturing, not consuming, lets a vertical sweep become a scroll; a still tap arms on RELEASE.
	h.arb.begin(y, h.ownerGrid())
	return InputCaptured
}

// Past the tap slop the gesture is a scroll and the release will not arm.
func (h *synthControlHitAdapter) OnDrag(x, y int) {
	if h.isArmed() {
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

// A still click anywhere on the armed card locks it; a spin stays armed. Not armed, only a tap arms, so a
// scroll leaves no stray selection change.
func (h *synthControlHitAdapter) OnRelease(x, y int) {
	if h.isArmed() {
		if h.dv.instEditorArmedEditor != nil {
			h.dv.instEditorArmedEditor.EndDrag()
		}
		if !h.armedMoved {
			h.dv.toggleSynthControlArm(h.instID, h.idx)
		}
		return
	}
	if !h.arb.end() {
		return
	}
	h.dv.armSynthControl(h.instID, h.idx)
}

// Not armed, a wheel over a control scrolls, never edits; armed, a spin is ONE debounced undo step.
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

// body=true is the empty-space grab (smooth, content follows the finger); false is the thumb (stepped, the
// thumb follows it). Created per Layout so it never caches a stale grid.
type synthSectionScrollAdapter struct {
	dv   *DrumView
	id   synthSectionID
	body bool
	// Otherwise the adapter resolves the stage grid by id.
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
	// Always consumed, so the wheel never leaks to a lower handler over the card.
	if g.WheelStep(steps) {
		h.invalidate()
	}
	return InputConsumed
}

type synthSliderHitAdapter = synthControlHitAdapter

// The frame adapter consumes clicks inside the dialog so they never reach the tab beneath.
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

// The same path as a chip tap, so the JS bridge and scenes drive what a click drives. Always false in production.
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

func (p *synthPanel) SynthTabNoSynth() bool {
	dv := p.dv
	return dv.instEditorNoSynth
}

// SynthTabBannerRect exposes the no-synth banner geometry for tests.
func (p *synthPanel) SynthTabBannerRect() image.Rectangle {
	dv := p.dv
	return dv.instEditorBannerRect
}

func (p *synthPanel) SynthTabHeaderButtons() []*Button {
	dv := p.dv
	return dv.instEditorHeaderBtns
}

func (p *synthPanel) SynthSaveAsDialog() *synthSaveAsDialog {
	dv := p.dv
	return dv.saveAsDialog
}

// Drives Save As's Primary/Secondary spec.
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

func (s synthSection) KnobCount() int { return len(s.knobIdxs) }

// Rect returns the section card's bounding rectangle (test-only).
func (s synthSection) Rect() image.Rectangle { return s.rect }

func (s synthSection) EnableParam() string { return s.enableParam }

func (s synthSection) KnobIdxs() []int { return s.knobIdxs }

// Keeps the fmt+strings imports from being dropped when the code above changes.
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

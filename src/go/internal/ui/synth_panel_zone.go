package ui

import (
	"fmt"
	"image"
	"image/color"
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
	// VOICE section is empty and omitted. No enable pill — the voice is the
	// instrument.
	synthSectionVoice
	// Phase-8C modulator stages (the spec-§1 gap closure): PITCH ENV reuses
	// synthSectionPitch (same label/gloss); LFO and BURST get their own IDs.
	// All three default DISABLED on every recipe (new DSP; off = the exact
	// pre-Phase-8C render), with audible knob defaults behind the pill.
	synthSectionLFO
	synthSectionBurst
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
//	VOICE · OSC · FM · PITCH · LFO · BURST · ENVELOPE · FILTER · POST
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
var unifiedSynthSectionOrder = []synthSectionID{
	synthSectionVoice,
	synthSectionOsc,
	synthSectionFM,
	synthSectionPitch,
	synthSectionLFO,
	synthSectionBurst,
	synthSectionEnvelope,
	synthSectionFilter,
	synthSectionPost,
}

// synthSectionOrderForSchema returns the standardized section order for the
// given recipe schema, pruning empty sections. Every synth instrument is laid
// out with the same VOICE·OSC·FM·ENVELOPE·FILTER·POST sequence; only sections
// that have at least one knob or an enable pill survive. POST always survives
// (every recipe carries gain + post_enabled, or a generic post knob).
func synthSectionOrderForSchema(schema []audio.ParamDef) []synthSectionID {
	recipeID := schemaRecipeID(schema)
	// Tally per-section content: a knob (grid param) or an enable pill (a
	// per-stage *_enabled toggle that is actually appended to this recipe).
	hasContent := map[synthSectionID]bool{}
	for _, d := range schema {
		if d.Group == audio.SynthHiddenGroup {
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
	case synthSectionVoice:
		return "VOICE"
	case synthSectionLFO:
		return "LFO"
	case synthSectionBurst:
		return "BURST"
	}
	return ""
}

// sectionSubtitle returns a kid-friendly secondary label rendered under
// the jargon section title (Phase 4 / 5 of the audio-panel redesign).
// Always-on — both labels visible so the section card teaches the
// vocabulary without hiding the professional term.
func sectionSubtitle(id synthSectionID) string {
	switch id {
	case synthSectionPitch:
		return "how high or low"
	case synthSectionEnvelope:
		return "how it starts and dies"
	case synthSectionTone:
		return "bright or dull"
	case synthSectionPost: // == synthSectionDrive
		return "shape the finished sound"
	case synthSectionOsc:
		return "the raw waveform"
	case synthSectionFM:
		return "metallic harmonics"
	case synthSectionFilter:
		return "carve the tone"
	case synthSectionVoice:
		return "what makes this sound"
	case synthSectionLFO:
		return "wobble the loudness"
	case synthSectionBurst:
		return "rapid-fire hits"
	}
	return ""
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
	case "drive_enabled", "post_enabled":
		return synthSectionPost, true
	// Phase-8C modulator stages. PITCH ENV reuses the PITCH section ID.
	case "pitchenv_enabled":
		return synthSectionPitch, true
	case "lfo_enabled":
		return synthSectionLFO, true
	case "burst_enabled":
		return synthSectionBurst, true
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
	case "post":
		return synthSectionPost, true
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

// synthParamIsGridKnob reports whether a ParamDef is shown as a knob in the
// section grid. Engine-internal (hidden) params and the per-stage enable
// toggles are excluded — the toggles render as section pills, and hidden
// params (noise_seed) have no UI at all.
func synthParamIsGridKnob(def audio.ParamDef) bool {
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
	recipeID := audio.RecipeForInstrument(instID)
	merged := audio.MergeRecipeDefaults(recipeID, audio.GetInstrumentParams(instID))
	v, ok := merged[paramName]
	if !ok {
		return true
	}
	return v >= 0.5
}

// toggleSynthStage flips a modular per-stage bypass toggle. Routes through
// the same SetInstrumentParam plumbing as a knob drag (coalesced, persisted,
// voice-cache invalidated) — the change is config-only and the NEXT trigger
// of the instrument renders with the stage flipped. No automatic audition:
// an immediate full-gain one-shot layered over the sequencer's scheduled
// voices was heard as a crackle/sharp transient during playback. The
// explicit Preview button is the only audition path.
func (dv *DrumView) toggleSynthStage(instID, paramName string) {
	instID = dv.resolveSynthInstrument(instID)
	if instID == "" || paramName == "" {
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

// synthSection is one card in the section row. Empty knobIdxs means the
// section has no wired knobs for the current recipe and the card is
// rendered as a collapsed placeholder.
type synthSection struct {
	id       synthSectionID
	rect     image.Rectangle
	knobIdxs []int // indices into instEditorKnobs / instEditorBindings
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
func (dv *DrumView) selectedSynthSection(instID string, sections []synthSection) synthSectionID {
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
func (dv *DrumView) setSelectedSynthSection(instID string, id synthSectionID) {
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
func (dv *DrumView) synthSelectedSection() *synthSection {
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
// carries no enable toggle (VOICE) or the pane is too small. Replaces the
// per-card sectionEnablePillRect placement that used to collide with titles
// and knobs on narrow cards.
func (dv *DrumView) synthDetailEnablePillRect() image.Rectangle {
	sel := dv.synthSelectedSection()
	detail := dv.instEditorDetailR
	if sel == nil || sel.enableParam == "" || detail.Empty() {
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
	// overflowRect is the chevron tap target surfaced when the cascade
	// collapses one or more of Preview / Save / SaveAs / Reset. Tapping it
	// opens a bottom sheet listing the dropped actions. Empty when nothing
	// was collapsed. Phase 2 audio-panel redesign.
	overflowRect    image.Rectangle
	overflowActions []string // "preview", "save", "save-as", "reset"
	instLabel       string
	recipeID        string
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

// buildSynthTab is the layout entry point invoked by EQPanelZone when
// TabSynth is active. Computes header / section / OUT column / footer
// rects, populates instEditorKnobs + instEditorSliders + instEditorBindings
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
func (dv *DrumView) buildSynthTab(contentR image.Rectangle, instID string) {
	dv.instEditorBtns = dv.instEditorBtns[:0]
	dv.instEditorSections = dv.instEditorSections[:0]
	dv.instEditorHeader = synthHeaderLayout{}
	dv.instEditorNoSynth = false
	dv.instEditorBannerRect = image.Rectangle{}
	if contentR.Empty() {
		dv.instEditorKnobs = dv.instEditorKnobs[:0]
		dv.instEditorSliders = dv.instEditorSliders[:0]
		dv.instEditorStepBadges = dv.instEditorStepBadges[:0]
		dv.instEditorBindings = dv.instEditorBindings[:0]
		return
	}

	resolved := dv.resolveSynthInstrument(instID)
	if resolved == "" {
		dv.instEditorKnobs = dv.instEditorKnobs[:0]
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
		dv.instEditorKnobs = dv.instEditorKnobs[:0]
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

	genMerged := audio.MergeRecipeDefaults(recipeID, audio.GetInstrumentParams(resolved))
	deferred := false
	// Defer any schema CHANGE while a knob drag is in flight. A schema swap
	// mid-gesture (historically the gen_type Native→waveform re-voice; today
	// a recipe rebind from any source) rebuilds the binding slice while the
	// drag is captured BY INDEX — the tree routes continued OnDrag to the
	// captured handler without re-testing hit areas, so the captured index
	// would bind a DIFFERENT param and the in-flight drag would scramble it
	// (in WASM that silently drove foreign params like gain/osc_enabled,
	// silencing the voice until reload). Hold the current knob set until the
	// gesture releases; the next Layout swaps cleanly. The adapter also
	// self-defends by latching the captured param name — see
	// synth_knob_adapter_index_safe_test.go.
	capturingNow := dv.anySynthKnobCapturing()
	if capturingNow && !bindingsMatchSchema(dv.instEditorBindings, schema) {
		schema = schemaFromBindings(dv.instEditorBindings)
		deferred = true
	}
	if synthDragDebug {
		dv.sdbgBuildTab(fmt.Sprintf("inst=%s deferred=%v capturing=%v prevBindings=%d schemaLen=%d",
			resolved, deferred, capturingNow, len(dv.instEditorBindings), len(schema)))
	}

	p := Profile()
	mobile := p.IsMobile()
	headerH := Profile().SynthHeaderH
	// Adaptive shrink: when the panel is short, shrink the header so
	// the section row keeps at least minSectionsH for the knobs. The
	// Save / SaveAs / Reset buttons live inside the header now, so
	// there's no separate footer band to reserve — sections own all
	// vertical space below the header.
	const minSectionsH = 60
	if contentR.Dy() < headerH+minSectionsH {
		headerH = contentR.Dy() - minSectionsH
		if headerH < 0 {
			headerH = 0
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
		// Mobile: no side pane. ALWAYS reserve a COMPACT stacked band at the top
		// of the sections area for the focus graph (+ the "Your sound" mirror when
		// the band is tall enough — splitSynthRightPane collapses the mirror
		// gracefully on short bands, leaving the focus graph the whole band). The
		// band is never empty on mobile so the selected knob is always explained;
		// the chip strip + detail/knob grid consumes the shrunk sectionsRect below
		// and SCROLLS so every knob stays reachable (selectSectionForKnobIdx /
		// ControlGrid.ScrollToIndex bring an off-screen knob into view on select).
		//
		// Sizing: aim for the focus-graph token height, but never let the band
		// exceed 45% of the sections area, and always leave the knob grid a
		// usable floor (chip strip — which may wrap — + the detail header + one
		// knob row). On a tiny panel the band shrinks to a still-legible minimum
		// (splitSynthRightPane drops the mirror and gives the focus graph the
		// whole band) rather than starving the grid.
		const (
			minBandH = 64  // focus graph alone stays legible (mirror collapses)
			minGridH = 150 // wrapped chip strip + detail header + a knob row
		)
		sectionsDy := sectionsRect.Dy()
		capBandH := sectionsDy * 45 / 100
		bandH := Profile().DensityValues().SynthFocusGraphH
		if bandH > capBandH {
			bandH = capBandH
		}
		// Protect the grid floor: if reserving the band would push the grid below
		// minGridH, shrink the band — but keep it at least minBandH so the focus
		// graph never vanishes (only when the whole sections area is smaller than
		// minBandH+minGridH does the band drop below minBandH).
		if sectionsDy-bandH < minGridH {
			bandH = sectionsDy - minGridH
			if bandH < minBandH {
				bandH = minBandH
			}
		}
		if bandH > sectionsDy {
			bandH = sectionsDy
		}
		if bandH < 1 {
			bandH = 1
		}
		dv.instEditorMobileFocusRect = image.Rect(
			sectionsRect.Min.X, sectionsRect.Min.Y,
			sectionsRect.Max.X, sectionsRect.Min.Y+bandH,
		)
		sectionsRect.Min.Y = dv.instEditorMobileFocusRect.Max.Y + SpaceSM
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
		dv.sdbg("[synthdrag] buildSynthTab REBUILDING knob slice (reuseExisting=false) — captured drag state will be LOST if a drag is in flight; capturing=%v", dv.anySynthKnobCapturing())
	}
	if !reuseExisting {
		dv.instEditorKnobs = dv.instEditorKnobs[:0]
		dv.instEditorSliders = dv.instEditorSliders[:0]
		dv.instEditorStepBadges = dv.instEditorStepBadges[:0]
		dv.instEditorBindings = dv.instEditorBindings[:0]
	}
	for i, def := range schema {
		sec := sectionForParamDefIn(recipeID, def)
		switch {
		case def.Group == audio.SynthHiddenGroup:
			// Engine-internal (noise_seed) — no UI. Still allocate a widget
			// slot below so knob/binding indices stay aligned with the schema.
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
		if reuseExisting && i < len(dv.instEditorKnobs) {
			// Keep the existing knob/slider — preserve drag state,
			// pressY latch, captured-handler pointer. Only refresh the
			// Value when NO drag is in progress; mid-drag, the user's
			// finger owns the value and we must not clobber it.
			if !dv.instEditorKnobs[i].Capturing() {
				dv.instEditorKnobs[i].Value = initial
				dv.instEditorSliders[i].Value = initial
			}
			dv.instEditorKnobs[i].Bipolar = bipolar
			dv.instEditorKnobs[i].ZeroFrac = zeroFrac
			dv.instEditorKnobs[i].Scale = sc
			dv.instEditorKnobs[i].Discrete = discrete
			dv.instEditorKnobs[i].Endless = endless
			// Refresh StepMul from the existing badge when present.
			if endless && i < len(dv.instEditorStepBadges) && dv.instEditorStepBadges[i] != nil {
				dv.instEditorKnobs[i].StepMul = dv.instEditorStepBadges[i].Step()
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
		dv.instEditorKnobs = append(dv.instEditorKnobs, k)
		dv.instEditorStepBadges = append(dv.instEditorStepBadges, badge)
		dv.instEditorSliders = append(dv.instEditorSliders, NewSlider(initial))
	}

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

// bindingsMatchSchema reports whether the existing bindings line up with
// the new schema by length + param name. Used by buildSynthTab to decide
// whether to reuse existing knob/slider instances (preserving drag state)
// or rebuild the slice from scratch (when the active recipe changed).
// anySynthKnobCapturing reports whether a synth-tab knob drag is currently in
// flight. Used to defer schema swaps so a captured-by-index drag handler is not
// re-bound to a different param mid-gesture (see buildSynthTab).
func (dv *DrumView) anySynthKnobCapturing() bool {
	for _, k := range dv.instEditorKnobs {
		if k != nil && k.Capturing() {
			return true
		}
	}
	return false
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
func (dv *DrumView) layoutSynthHeader(contentR image.Rectangle, headerH int, instID, recipeID string, mobile bool) {
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

	// Action buttons (Preview, Save, Save As, Reset) sit immediately right
	// of the caption text in the top-left quadrant of the panel. Widths fit
	// the labels (no truncation) and the height is the density token so
	// mobile (Spacious) buttons meet the 44-px touch-min. Height is clamped
	// to the header's inner band so it never exceeds the strip.
	previewW := labelButtonWidth(i18n.T(i18n.KeyPreview))
	saveW := labelButtonWidth(i18n.T(i18n.KeySave))
	saveAsW := labelButtonWidth(i18n.T(i18n.KeySaveAs))
	resetW := labelButtonWidth(i18n.T(i18n.KeyReset))
	btnH := dv2.SynthHeaderButtonH
	if innerH := innerY1 - innerY0; innerH > 0 && btnH > innerH {
		btnH = innerH
	}
	btnY0 := (r.Min.Y+r.Max.Y)/2 - btnH/2
	btnY1 := btnY0 + btnH

	captionLeft := waveformR.Max.X + SpaceSM
	rightEdge := r.Max.X - inset
	totalBtnW := previewW + SpaceSM + saveW + SpaceSM + saveAsW + SpaceSM + resetW
	// Caption gets whatever fits between the waveform and the button
	// group, with a comfortable upper bound on wide panels so the
	// buttons stay grouped near the top-left rather than drifting to
	// the right edge.
	captionMaxW := 240
	if mobile {
		captionMaxW = 200
	}
	captionRight := rightEdge - totalBtnW - SpaceMD
	if captionRight > captionLeft+captionMaxW {
		captionRight = captionLeft + captionMaxW
	}
	if captionRight < captionLeft+40 {
		captionRight = captionLeft + 40
	}
	captionR := image.Rect(captionLeft, innerY0, captionRight, innerY1)

	// Place buttons a SpaceMD gap right of the caption. Preview leads the
	// group (mirrors the Sampler tab's preview-then-save flow).
	btnX := captionR.Max.X + SpaceMD
	previewR := image.Rect(btnX, btnY0, btnX+previewW, btnY1)
	saveR := image.Rect(previewR.Max.X+SpaceSM, btnY0, previewR.Max.X+SpaceSM+saveW, btnY1)
	saveAsR := image.Rect(saveR.Max.X+SpaceSM, btnY0, saveR.Max.X+SpaceSM+saveAsW, btnY1)
	resetR := image.Rect(saveAsR.Max.X+SpaceSM, btnY0, saveAsR.Max.X+SpaceSM+resetW, btnY1)

	// Overflow chevron — surfaces collapsed actions in a bottom sheet so
	// the Save button is never silently lost on narrow panels. Reserve
	// width up-front; only actually drawn when the cascade collapses at
	// least one action. Phase 2 audio-panel redesign.
	chevronW := btnH
	if chevronW < 28 {
		chevronW = 28
	}
	rightEdgeBudget := rightEdge // for full layout (no chevron)
	rightEdgeWithChev := rightEdge - chevronW - SpaceSM

	// Cascade: collapse Preview first (least load-bearing — knob release
	// already auditions), then Save As, then Save. Reset stays as long as
	// possible (load-bearing action). Each collapse reserves the chevron
	// rect; if everything fits, the chevron is suppressed.
	var dropped []string
	if resetR.Max.X > rightEdgeBudget {
		// First collapse: hide Preview and re-pack the group at btnX.
		previewR = image.Rectangle{}
		dropped = append(dropped, "preview")
		saveR = image.Rect(btnX, btnY0, btnX+saveW, btnY1)
		saveAsR = image.Rect(saveR.Max.X+SpaceSM, btnY0, saveR.Max.X+SpaceSM+saveAsW, btnY1)
		resetR = image.Rect(saveAsR.Max.X+SpaceSM, btnY0, saveAsR.Max.X+SpaceSM+resetW, btnY1)
		if resetR.Max.X > rightEdgeWithChev {
			// Second collapse: hide Save As.
			saveAsR = image.Rectangle{}
			dropped = append(dropped, "save-as")
			resetR = image.Rect(saveR.Max.X+SpaceSM, btnY0, saveR.Max.X+SpaceSM+resetW, btnY1)
			if resetR.Max.X > rightEdgeWithChev {
				// Third collapse: hide Save too. Reset stays.
				saveR = image.Rectangle{}
				dropped = append(dropped, "save")
				resetR = image.Rect(btnX, btnY0, btnX+resetW, btnY1)
				if resetR.Max.X > rightEdgeWithChev {
					// Final collapse: even Reset doesn't fit. Surface all four
					// through the chevron — never silently drop any.
					resetR = image.Rectangle{}
					dropped = append(dropped, "reset")
				}
			}
		}
	}
	overflowR := image.Rectangle{}
	if len(dropped) > 0 {
		overflowR = image.Rect(rightEdge-chevronW, btnY0, rightEdge, btnY1)
	}

	dv.instEditorHeader = synthHeaderLayout{
		rect:            r,
		waveformRect:    waveformR,
		captionRect:     captionR,
		resetRect:       resetR,
		saveRect:        saveR,
		saveAsRect:      saveAsR,
		previewRect:     previewR,
		overflowRect:    overflowR,
		overflowActions: dropped,
		instLabel:       instID,
		recipeID:        recipeID,
	}
	dv.instEditorHeaderBtns = dv.instEditorHeaderBtns[:0]
}

// buildSynthHeaderButtons constructs the three *Button widgets that sit
// inside the header strip. Save's spec toggles Primary↔Secondary by
// dirty state so the visual signal stays honest. Empty rects (very
// narrow panels) just skip the corresponding button entirely.
func (dv *DrumView) buildSynthHeaderButtons(resolved string) {
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
	if !h.overflowRect.Empty() {
		actions := append([]string(nil), h.overflowActions...)
		overflowBtn := NewButton("", InstButtonStyle, func() {
			dv.openSynthOverflowSheet(actions, resolved)
		})
		overflowBtn.SetRect(h.overflowRect)
		overflowBtn.Icon = string(IconOverflow)
		overflowBtn.IconColor = colTextSecondary
		dv.instEditorBtns = append(dv.instEditorBtns, overflowBtn)
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
func (dv *DrumView) layoutSynthSections(rowR image.Rectangle, sections []synthSection, mobile bool, instID string) {
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
		const minDetailH = 72 // detail header + one knob row (knobs scroll)
		if rows := synthChipWrapRows(sections, rowR, chipMinW, gap); rows > 1 {
			avail := rowR.Dy() - minDetailH - SpaceSM
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
	dv.instEditorDetailR = detailR
	// Effective detail-header height: the full token when the pane is tall
	// enough for header + one ideal knob row; otherwise shrink toward the
	// bare title row so the knobs (the actual controls) keep their space.
	headerH := dv2.SynthDetailHeaderH
	if knobRowMin := dv2.SynthKnobMin + dv2.SynthKnobCaptionH + 2*synthSectionPaddingY; detailR.Dy() < headerH+knobRowMin {
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
func (dv *DrumView) appendSynthChip(s synthSection, r image.Rectangle, instID string, selID synthSectionID) {
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
	w := int(float64(TextWidth(sectionLabel(id)))*captionScale) + 2*SpaceSM + synthChipStateW
	if w < minW {
		w = minW
	}
	return w
}

// sectionGrid returns the persistent ControlGrid for a section id, creating it
// on first use. Keyed by id (not slice position) so the scroll position
// survives a recipe switch that reorders the sections.
func (dv *DrumView) sectionGrid(id synthSectionID) *ControlGrid {
	if dv.instEditorSectionGrids == nil {
		dv.instEditorSectionGrids = map[synthSectionID]*ControlGrid{}
	}
	g := dv.instEditorSectionGrids[id]
	if g == nil {
		g = NewControlGrid(ScrollbarStyleForPlatform())
		dv.instEditorSectionGrids[id] = g
	}
	return g
}

// synthTabUpdate ticks the per-frame scroll cooldown clock for every section
// grid (see ControlGrid.WheelStep / Tick). Also drives the shared numeric
// editor (paramEditor) so blur-to-commit works in production (mirrors
// TransportZone.Update pumping the BPM box every frame). Returns true when
// the layout should be re-derived (e.g. editor opened/closed).
func (dv *DrumView) synthTabUpdate() bool {
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
// diameter that fits the cell, clamped to [SynthKnobMin, SynthKnobIdeal]. The
// slider's rect mirrors the knob's so legacy hit-tests + JS export rects keep
// landing on the visible widget.
// synthKnobCellHeight is the per-knob grid cell height: dial + caption + gap.
// The old per-knob concept-viz band is gone (the focus graph explains the
// selected knob in the right pane), so the cell no longer reserves SynthConceptVizH.
func synthKnobCellHeight() int {
	d := Profile().DensityValues()
	return d.SynthKnobIdeal + d.SynthKnobCaptionH + SpaceXS
}

func (dv *DrumView) placeKnobsInSection(s synthSection, mobile bool) {
	grid := dv.sectionGrid(s.id)
	// Collapsed stage (empty section rect) or no knobs: clear every knob
	// rect explicitly. Beware image.Rect's coordinate normalization — deriving
	// innerR from an empty s.rect would swap the negative spans into a small
	// NON-empty rect near the origin and leak phantom knob rects.
	if len(s.knobIdxs) == 0 || s.rect.Empty() {
		grid.Layout(image.Rectangle{}, 0, 1, 1, 0, 0)
		for _, kIdx := range s.knobIdxs {
			if kIdx < 0 || kIdx >= len(dv.instEditorKnobs) {
				continue
			}
			dv.instEditorKnobs[kIdx].SetRect(image.Rectangle{})
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
	// to SynthKnobIdeal and placed at the TOP of the slot, so the taller cell
	// does NOT enlarge the dial or change its hit geometry — it just frees the
	// lower slot for the viz band, leaving left/right knob drag unaffected.
	// Shrinks to the card when even one ideal row won't fit.
	// The plain-English purpose gloss ("bright or dull", …) is drawn on the
	// caption ROW beside the value (see drawSynthDetailPane / synthKnobPurpose) —
	// the fewer-per-row cap makes cells wide enough for both — so it costs no
	// vertical space here; the cell only reserves dial + caption + concept band.
	cellH := synthKnobCellHeight()
	if availH := innerR.Dy(); cellH > availH {
		cellH = availH
	}
	if cellH < 1 {
		cellH = 1
	}
	// Fewer, WIDER knobs per row so each cell has room for its purpose line +
	// mini-visual: cap the grid to 2 columns on desktop, 1 on mobile (the cap
	// overrides ControlGrid's 2-column floor). Overflow scrolls — the intended
	// "show a few, scroll for more" behaviour.
	maxCols := 2
	if mobile {
		maxCols = 1
	}
	grid.SetMaxCols(maxCols)
	grid.Layout(innerR, len(s.knobIdxs), dv2.SynthKnobIdeal, cellH, hGap, vGap)

	for i, kIdx := range s.knobIdxs {
		slot, vis := grid.CellRect(i)
		if !vis {
			dv.instEditorKnobs[kIdx].SetRect(image.Rectangle{})
			dv.instEditorSliders[kIdx].SetRect(image.Rectangle{})
			continue
		}
		// Largest knob that fits the cell, clamped to the density bounds. The fit
		// reserves the caption row AND the concept-viz band below it, so the dial
		// yields room for its mini-picture — but never below SynthKnobMin (a
		// usability/touch invariant, guarded by TestSynthSection_*MinDiameter).
		// The synth panel is sized tall enough (see PanelTabState.PanelHeightAt)
		// that a full row of dial + caption + band fits without clamping.
		d := slot.Dx()
		if maxH := slot.Dy() - dv2.SynthKnobCaptionH; d > maxH {
			d = maxH
		}
		if d > dv2.SynthKnobIdeal {
			d = dv2.SynthKnobIdeal
		}
		if d < dv2.SynthKnobMin {
			d = dv2.SynthKnobMin
		}
		x0 := slot.Min.X + (slot.Dx()-d)/2
		rect := image.Rect(x0, slot.Min.Y, x0+d, slot.Min.Y+d+dv2.SynthKnobCaptionH)
		dv.instEditorKnobs[kIdx].SetRect(rect)
		dv.instEditorSliders[kIdx].SetRect(rect)
	}
}

// drawSynthTab renders the prebuilt section model. Called by EQPanelZone.Draw
// when activeTab == TabSynth.
func (dv *DrumView) drawSynthTab(dst *ebiten.Image, contentR image.Rectangle, instID string) {
	if contentR.Empty() {
		return
	}
	resolved := dv.resolveSynthInstrument(instID)
	if resolved == "" {
		// No active row — draw a hint and return.
		DrawTextColorAt(dst, i18n.T(i18n.KeyNoRowSelected), contentR.Min.X+SpaceMD, contentR.Min.Y+SpaceMD, TokenTextSecondary())
		return
	}

	// Sample (WAV) instrument: no synth controls to shape. Paint the single
	// explanatory banner instead of the header + section grid + footer.
	if dv.instEditorNoSynth {
		drawSynthNoSynthBanner(dst, dv.instEditorBannerRect)
		return
	}

	dv.drawSynthHeader(dst)
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
		mirrorR, focusR := splitSynthRightPane(r)
		if !mirrorR.Empty() {
			drawSynthMirror(dst, mirrorR, dv.synthMirror)
		}
		dv.drawSynthFocusGraph(dst, focusR, resolved)
	}
	// Mobile: no side pane — the "Your sound" mirror + focus graph live in a
	// stacked band reserved at the top of the sections area instead.
	if mb := dv.instEditorMobileFocusRect; !mb.Empty() {
		dv.updateSynthMirrorLive(resolved)
		mirrorR, focusR := splitSynthRightPane(mb)
		if !mirrorR.Empty() {
			drawSynthMirror(dst, mirrorR, dv.synthMirror)
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
func (dv *DrumView) synthDecayValueForPreview() float64 {
	for i, b := range dv.instEditorBindings {
		if b.def.Name != "decay" || i >= len(dv.instEditorKnobs) {
			continue
		}
		k := dv.instEditorKnobs[i]
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
func (dv *DrumView) drawSynthOutRow(dst *ebiten.Image, paneR image.Rectangle, instID string) {
	const rowH = 20
	if paneR.Dy() < 80 {
		return
	}
	rowR := image.Rect(paneR.Min.X+SpaceSM, paneR.Max.Y-rowH-SpaceSM, paneR.Max.X-SpaceSM, paneR.Max.Y-SpaceSM)
	drawRect(dst, rowR, WithAlpha(genColorVizScopeBg, AlphaOverlay), true)

	captionScale := FontSizeCaption / FontSizeBody
	captionH := int(float64(TextHeight()) * captionScale)
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
		DrawTextColorAtScale(dst, item.label, cellR.Min.X+2, cellR.Min.Y+1, TokenTextSecondary(), captionScale)
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
func (dv *DrumView) drawSynthHeader(dst *ebiten.Image) {
	h := dv.instEditorHeader
	if h.rect.Empty() {
		return
	}
	// Background card.
	drawRoundedRect(dst, h.rect, TokenSurface1(), RadiusSM, true)
	drawRoundedRect(dst, h.rect, TokenBorderSubtle(), RadiusSM, false)

	// Caption: "snare  —  drum-snare", truncated to its rect so it never
	// runs under the Save/Save As/Reset buttons on a narrow (mobile) header.
	DrawTextColorAt(dst, synthHeaderCaptionText(h), h.captionRect.Min.X, h.captionRect.Min.Y+SpaceSM, TokenTextPrimary())

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
	return truncCaption(caption, h.captionRect.Dx())
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
// drawSynthChipStrip paints the pipeline chip strip: one compact chip per
// stage in audio order, connected by a wire so the strip reads as the signal
// path. Selected chip = accent border + tinted fill; disabled stage = dimmed
// ghost chip with a power-ring affordance (IconCircle outline). Connectors
// are drawn geometry — the forbidden-glyph rule bans `→`/`▶` text.
func (dv *DrumView) drawSynthChipStrip(dst *ebiten.Image) {
	captionScale := FontSizeCaption / FontSizeBody
	for i, c := range dv.instEditorChips {
		if c.rect.Empty() {
			continue
		}
		// Connector: a continuous thin AlphaFaint line running the full gap
		// between the previous chip's right edge and this chip's left edge,
		// vertically centred. Drawn BEFORE the chip body so the pill always
		// sits on top — the wire never glues to a label.
		if i > 0 {
			p := dv.instEditorchipPrev(i)
			if !p.rect.Empty() && p.rect.Min.Y == c.rect.Min.Y && p.rect.Max.X < c.rect.Min.X {
				cy := (c.rect.Min.Y + c.rect.Max.Y) / 2
				wire := image.Rect(p.rect.Max.X, cy, c.rect.Min.X, cy+1)
				drawRect(dst, wire, WithAlpha(genColorBorder, AlphaFaint), true)
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
		drawRoundedRect(dst, c.rect, fill, RadiusXS, true)
		drawRoundedRect(dst, c.rect, border, RadiusXS, false)
		// State dot right of the label (only for stages that carry an enable
		// toggle). >= 6px, explicit on/off colors: accent when on, disabled
		// surface-text when off.
		if c.enableParam != "" {
			const dotD = 7
			cy := (c.rect.Min.Y + c.rect.Max.Y) / 2
			dotR := image.Rect(
				c.rect.Max.X-SpaceSM-dotD, cy-dotD/2,
				c.rect.Max.X-SpaceSM, cy-dotD/2+dotD,
			)
			if c.enabled {
				DrawIcon(dst, IconCircle, dotR, colAccent)
			} else {
				DrawIcon(dst, IconCircle, dotR, TokenTextDisabled())
			}
		}
		// Label, truncated to the room left of the state affordance.
		availPx := c.rect.Dx() - 2*SpaceSM - synthChipStateW
		if availPx <= 0 {
			continue
		}
		label := truncCaption(sectionLabel(c.id), int(float64(availPx)/captionScale))
		labelH := int(float64(TextHeight()) * captionScale)
		ly := c.rect.Min.Y + (c.rect.Dy()-labelH)/2
		DrawTextColorAtScale(dst, label, c.rect.Min.X+SpaceSM, ly, labelCol, captionScale)
	}
}

// instEditorchipPrev returns the chip immediately before index i, used by the
// chip-strip connector. Small helper so the connector logic reads clearly.
func (dv *DrumView) instEditorchipPrev(i int) synthChip {
	return dv.instEditorChips[i-1]
}

// drawSynthDetailPane paints the expanded stage: a header band (title +
// plain-English subtitle + enable pill, now with room so nothing overlaps)
// over the full-size knob grid. The bypass scrim dims the knob area when the
// selected stage is toggled off — the pill stays bright so re-enabling is
// obvious; knobs remain interactive (tweaking a bypassed stage is allowed,
// it just has no effect until re-enabled).
func (dv *DrumView) drawSynthDetailPane(dst *ebiten.Image, instID string) {
	detail := dv.instEditorDetailR
	sel := dv.synthSelectedSection()
	if detail.Empty() || sel == nil {
		return
	}
	drawRoundedRect(dst, detail, TokenSurface1(), RadiusSM, true)
	drawRoundedRect(dst, detail, TokenBorderSubtle(), RadiusSM, false)

	headerH := dv.instEditorDetailHeaderH
	stageEnabled := true
	if sel.enableParam != "" {
		stageEnabled = synthStageEnabled(instID, sel.enableParam)
	}

	tx := detail.Min.X + synthSectionPaddingX
	ty := detail.Min.Y + SpaceXS
	// Title shares the body's dim state: when the stage is bypassed the title
	// dims too (it stayed full-brightness before, contradicting the dimmed
	// knobs) and gains a small "BYPASSED" tag so the off-state is unmistakable.
	titleCol := TokenTextPrimary()
	if !stageEnabled {
		titleCol = TokenTextDisabled()
	}
	title := sectionLabel(sel.id)
	DrawTextColorAt(dst, title, tx, ty, titleCol)
	if !stageEnabled {
		captionScale := FontSizeCaption / FontSizeBody
		tagX := tx + TextWidth(title) + SpaceSM
		DrawTextColorAtScale(dst, i18n.T(i18n.KeyCapBypassed), tagX, ty+2, TokenTextDisabled(), captionScale)
	}
	if sub := sectionSubtitle(sel.id); sub != "" {
		captionScale := FontSizeCaption / FontSizeBody
		subY := ty + TextHeight() + 1
		if subY+int(float64(TextHeight())*captionScale) <= detail.Min.Y+headerH {
			availW := detail.Dx() - 2*synthSectionPaddingX - FXToggleTrackW() - 2*SpaceXS
			sub = truncCaption(sub, int(float64(availW)/captionScale))
			subCol := WithAlpha(TokenTextSecondary(), AlphaMedium)
			if !stageEnabled {
				subCol = WithAlpha(TokenTextDisabled(), AlphaMedium)
			}
			DrawTextColorAtScale(dst, sub, tx, subY, subCol, captionScale)
		}
	}
	if sel.enableParam != "" {
		if pillR := dv.synthDetailEnablePillRect(); !pillR.Empty() {
			drawFXTogglePill(dst, pillR, stageEnabled)
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

	grid := dv.sectionGrid(sel.id)
	for j, kIdx := range sel.knobIdxs {
		if kIdx < 0 || kIdx >= len(dv.instEditorKnobs) {
			continue
		}
		k := dv.instEditorKnobs[kIdx]
		if k.Rect().Empty() {
			continue // scrolled out of the detail pane's visible window
		}
		k.Draw(dst)
		if dv.synthSelectedKnobIdx(instID, sel) == kIdx {
			drawRoundedRect(dst, k.Rect(), colConceptStroke, RadiusSM, false)
		}
		b := dv.instEditorBindings[kIdx]
		actual := b.def.Min + k.Value*(b.def.Max-b.def.Min)
		caption := synthKnobCaption(b.def, actual)
		captionR := k.Rect()
		slot, slotVis := grid.CellRect(j)
		if slotVis {
			captionR = image.Rect(slot.Min.X, captionR.Min.Y, slot.Max.X, captionR.Max.Y)
		}
		captionY := captionR.Min.Y + (k.Rect().Dy() - Profile().DensityValues().SynthKnobCaptionH) + SpaceXS

		// Resolve the step badge (continuous knobs only) and decide its layout
		// BEFORE drawing the caption, so the two never overlap. Preferred: a
		// pill BELOW the caption text (needs vertical room in the cell). When
		// the cell is too short, fall back to an INLINE pill at the right end
		// of the caption row and give the caption the remaining left width.
		var badge *KnobStepBadge
		if kIdx < len(dv.instEditorStepBadges) && slotVis {
			badge = dv.instEditorStepBadges[kIdx]
		}
		bw, bh := Profile().DensityValues().KnobStepBadgeW, Profile().DensityValues().KnobStepBadgeH
		badgeBelow := badge != nil && captionY+TextHeight()+SpaceXS+bh <= slot.Max.Y
		inlineBadge := badge != nil && !badgeBelow

		// Caption area: full cell width, minus the inline pill's column.
		capArea := captionR
		if inlineBadge {
			capArea.Max.X = captionR.Max.X - bw - SpaceXS
			if capArea.Max.X < capArea.Min.X {
				capArea.Max.X = capArea.Min.X
			}
		}
		// Plain-English purpose gloss ("bright or dull") shares the caption ROW,
		// right-aligned and dim, when the (now wide, fewer-per-row) cell can fit it
		// beside the value caption. Reserving the right slice here shrinks capArea
		// so the value caption + readout hit rect never overlap the gloss.
		glossScale := FontSizeCaption / FontSizeBody
		gloss := synthKnobPurpose(b.def)
		glossArea := image.Rectangle{}
		if gloss != "" {
			glossW := int(float64(TextWidth(gloss))*glossScale) + SpaceSM
			minCapW := TextWidth(synthKnobCaptionValueOnly(b.def, actual)) + SpaceXS*2
			if capArea.Dx()-glossW >= minCapW {
				glossArea = image.Rect(capArea.Max.X-glossW, captionY, capArea.Max.X, captionY+TextHeight())
				capArea.Max.X -= glossW
			}
		}
		availW := capArea.Dx() - SpaceXS*2
		if availW < 1 {
			availW = 1
		}
		if TextWidth(caption) > availW {
			caption = synthKnobCaptionValueOnly(b.def, actual)
		}
		caption = truncCaption(caption, availW)
		// Centre the caption within its area.
		cx := capArea.Min.X + (capArea.Dx()-TextWidth(caption))/2
		if cx < capArea.Min.X+2 {
			cx = capArea.Min.X + 2
		}
		DrawTextColorAt(dst, caption, cx, captionY, TokenTextSecondary())
		// Record the caption area as the tappable readout (stale rects cleared
		// above, so off-screen knobs keep an empty rect). In inline-badge mode
		// this excludes the pill's column so the two hit areas never overlap.
		if slotVis {
			readoutR := image.Rect(capArea.Min.X, captionY, capArea.Max.X, captionY+TextHeight())
			dv.setKnobReadoutRect(kIdx, readoutR)
		}
		// Purpose gloss on the caption row (right slice reserved above).
		if !glossArea.Empty() {
			gx := glossArea.Max.X - int(float64(TextWidth(gloss))*glossScale) - 2
			if gx < glossArea.Min.X {
				gx = glossArea.Min.X
			}
			glossCol := WithAlpha(TokenTextSecondary(), AlphaMedium)
			if !stageEnabled {
				glossCol = WithAlpha(TokenTextDisabled(), AlphaMedium)
			}
			DrawTextColorAtScale(dst, gloss, gx, captionY, glossCol, glossScale)
		}
		// Draw the step badge in its resolved position. Either way the rect
		// stays inside the cell slot so it can never reach into a neighbouring
		// cell and steal that knob's taps.
		if badge != nil {
			var br image.Rectangle
			if badgeBelow {
				by := captionY + TextHeight() + SpaceXS
				bx := captionR.Min.X + (captionR.Dx()-bw)/2
				br = image.Rect(bx, by, bx+bw, by+bh)
			} else {
				bx := captionR.Max.X - bw
				by := captionY + (TextHeight()-bh)/2
				if by < slot.Min.Y {
					by = slot.Min.Y
				}
				if by+bh > slot.Max.Y {
					by = slot.Max.Y - bh
				}
				br = image.Rect(bx, by, bx+bw, by+bh)
			}
			if !br.Empty() && br.In(slot) {
				badge.SetRect(br)
				badge.Draw(dst)
			}
		}

	}
	// Scrollbar when the stage's knobs overflow the pane.
	dv.sectionGrid(sel.id).Draw(dst)
	if sel.enableParam != "" && !stageEnabled {
		scrim := image.Rect(detail.Min.X+1, detail.Min.Y+headerH, detail.Max.X-1, detail.Max.Y-1)
		if !scrim.Empty() {
			drawRoundedRect(dst, scrim, WithAlpha(TokenSurface1(), AlphaStrong), RadiusXS, true)
		}
	}
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

	// fitScale shrinks a text scale down so the string fits availW; it never
	// scales up past the requested max. Floored so the text stays legible.
	fitScale := func(s string, max float64) float64 {
		w := TextWidth(s)
		if w <= 0 {
			return max
		}
		scale := max
		if fit := float64(availW) / float64(w); fit < scale {
			scale = fit
		}
		if scale < 0.6 {
			scale = 0.6
		}
		return scale
	}

	// Headline scaled up (capped at width); subtitle stays at body size but
	// shrinks to fit a narrow (mobile) panel so the hint is never dropped.
	headlineScale := fitScale(synthNoSynthHeadline, 1.6)
	subtitleScale := fitScale(synthNoSynthSubtitle, 1.0)
	headlineH := int(float64(TextHeight()) * headlineScale)
	subtitleH := int(float64(TextHeight()) * subtitleScale)

	// Vertically centre the headline + subtitle block.
	blockH := headlineH + SpaceSM + subtitleH
	top := cardR.Min.Y + (cardR.Dy()-blockH)/2
	if top < cardR.Min.Y+SpaceSM {
		top = cardR.Min.Y + SpaceSM
	}

	headlineW := int(float64(TextWidth(synthNoSynthHeadline)) * headlineScale)
	DrawTextColorAtScale(dst, synthNoSynthHeadline, cx-headlineW/2, top, TokenTextPrimary(), headlineScale)

	subW := int(float64(TextWidth(synthNoSynthSubtitle)) * subtitleScale)
	subY := top + headlineH + SpaceSM
	DrawTextColorAtScale(dst, synthNoSynthSubtitle, cx-subW/2, subY, TokenTextSecondary(), subtitleScale)
}

// drawSynthFooter renders the Reset / Save / Save-As buttons via the
// shared Button.Draw path. The buttons carry their NewSpecButton-assigned
// chrome (Primary/Secondary), so press/hover state, spec-driven theming,
// and visual consistency with the rest of the app come for free. Sentinel
// tags in btn.Text get translated to human labels at draw time so the
// `\x00synth-*` markers never leak to the user.
func (dv *DrumView) drawSynthFooter(dst *ebiten.Image) {
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
// label; "" for non-sentinel texts (e.g. the overflow chevron's empty text).
func synthHeaderButtonLabel(tag string) string {
	switch tag {
	case synthPreviewButtonTag:
		return i18n.T(i18n.KeyPreview)
	case synthSaveButtonTag:
		return i18n.T(i18n.KeySave)
	case synthSaveAsButtonTag:
		return i18n.T(i18n.KeySaveAs)
	case synthResetButtonTag:
		return i18n.T(i18n.KeyReset)
	}
	return ""
}

// synthKnobCaption formats one knob's caption as "name  value[ unit]"
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
	return def.Enum[idx]
}

func synthKnobCaption(def audio.ParamDef, value float64) string {
	// Discrete (Enum) params show only the selected label — the name lives in
	// the stage title; the waveform/algorithm name IS the value.
	if lbl := enumLabelFor(def, value); lbl != "" {
		return lbl
	}
	return synthParamDisplayName(def.Name) + "  " + formatSynthParamValue(def, value)
}

// synthKnobPurpose returns the one-line, kid-readable gloss for a knob ("bright
// or dull", "slight pitch shift", …) drawn under its caption. It prefers the
// param's display Label, falling back to its raw Name, and returns "" when
// neither maps (caller skips the line). Same PlainEnglish source the section
// subtitles use, applied per knob.
func synthKnobPurpose(def audio.ParamDef) string {
	if p := PlainEnglish(def.Label); p != "" {
		return p
	}
	return PlainEnglish(def.Name)
}

// synthKnobCaptionValueOnly returns just the value portion of the
// knob caption — the section title carries the parameter name, so on
// narrow mobile-grid cells where the full caption truncates with
// ellipsis we strip the name prefix and keep the value legible.
// Mobile-usability pass.
func synthKnobCaptionValueOnly(def audio.ParamDef, value float64) string {
	if lbl := enumLabelFor(def, value); lbl != "" {
		return lbl
	}
	return formatSynthParamValue(def, value)
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

// cycleKnobStep advances the step badge for knob idx and syncs the knob's
// StepMul, persisting the choice. Shared by the production hit adapter.
func (dv *DrumView) cycleKnobStep(idx int) {
	if idx < 0 || idx >= len(dv.instEditorStepBadges) {
		return
	}
	badge := dv.instEditorStepBadges[idx]
	if badge == nil {
		return
	}
	badge.Cycle()
	if idx < len(dv.instEditorKnobs) {
		dv.instEditorKnobs[idx].StepMul = badge.Step()
	}
	dv.persistKnobStep(badge)
}

// stepSynthKnobResolution shifts knob idx's step-badge rung by `steps` (the
// mouse-wheel delta; positive = scroll up = finer), syncing the knob's StepMul
// and persisting the choice. Returns true if the rung moved. Used by the wheel
// handler when the cursor is over the resolution badge.
func (dv *DrumView) stepSynthKnobResolution(idx, steps int) bool {
	if idx < 0 || idx >= len(dv.instEditorStepBadges) {
		return false
	}
	badge := dv.instEditorStepBadges[idx]
	if badge == nil || !badge.WheelResolution(steps) {
		return false
	}
	if idx < len(dv.instEditorKnobs) {
		dv.instEditorKnobs[idx].StepMul = badge.Step()
	}
	dv.persistKnobStep(badge)
	return true
}

// handleSynthTabInput dispatches a single mouse/touch event into the
// synth tab. Returns true when the event was consumed. Called by
// EQPanelZone.handleInput when activeTab == TabSynth.
func (dv *DrumView) handleSynthTabInput(x, y int, pressed bool, instID string) bool {
	instID = dv.resolveSynthInstrument(instID)
	// Active knob drag has priority.
	if dv.instEditorDragging {
		if dv.instEditorDragIdx < 0 || dv.instEditorDragIdx >= len(dv.instEditorKnobs) {
			dv.instEditorDragging = false
			return false
		}
		k := dv.instEditorKnobs[dv.instEditorDragIdx]
		result := k.HandleInputResult(x, y, pressed)
		if result == InputCaptured {
			dv.syncSliderFromKnob(dv.instEditorDragIdx)
			dv.propagateSynthSliderValue(dv.instEditorDragIdx, instID)
			return true
		}
		if result == InputConsumed {
			dv.syncSliderFromKnob(dv.instEditorDragIdx)
			dv.propagateSynthSliderValue(dv.instEditorDragIdx, instID)
			dv.instEditorDragging = false
			return true
		}
		dv.instEditorDragging = false
	}
	if !pressed {
		return false
	}
	// Knob hit-test.
	for i, k := range dv.instEditorKnobs {
		if image.Pt(x, y).In(k.Rect()) {
			k.HandleInputResult(x, y, pressed)
			dv.instEditorDragging = true
			dv.instEditorDragIdx = i
			dv.syncSliderFromKnob(i)
			dv.propagateSynthSliderValue(i, instID)
			return true
		}
	}
	// Selected stage's enable pill (detail-pane header). Hit-test before the
	// chips/footer so a header tap toggles the stage rather than falling
	// through.
	if sel := dv.synthSelectedSection(); sel != nil && sel.enableParam != "" {
		if pillR := dv.synthDetailEnablePillRect(); !pillR.Empty() && image.Pt(x, y).In(pillR) {
			dv.toggleSynthStage(instID, sel.enableParam)
			return true
		}
	}
	// Pipeline chips: a tap only OPENS (selects) the stage for inspection —
	// it never enables/disables it. Enabling a ghost (disabled) stage is an
	// explicit action via the detail-pane pill, so the user can look at a
	// disabled stage's knobs without turning it on.
	if !dv.anySynthKnobCapturing() {
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
func (dv *DrumView) syncSliderFromKnob(idx int) {
	if idx < 0 || idx >= len(dv.instEditorKnobs) || idx >= len(dv.instEditorSliders) {
		return
	}
	dv.instEditorSliders[idx].Value = dv.instEditorKnobs[idx].Value
}

// ---- Readout-rect helpers + numeric editor ------------------------------

// synthKnobReadoutRect returns the caption hit-rect for the given knob index.
// Returns the zero rectangle when the index is out of range or the rect has
// not been populated yet (no Draw pass has occurred).
func (dv *DrumView) synthKnobReadoutRect(idx int) image.Rectangle {
	if idx < 0 || idx >= len(dv.instEditorReadoutRects) {
		return image.Rectangle{}
	}
	return dv.instEditorReadoutRects[idx]
}

// setKnobReadoutRect stores the caption hit-rect for the given knob index,
// growing the slice as needed. Mirrors setKnobStepBadgeRect semantics from
// Task 11.
func (dv *DrumView) setKnobReadoutRect(idx int, r image.Rectangle) {
	for len(dv.instEditorReadoutRects) <= idx {
		dv.instEditorReadoutRects = append(dv.instEditorReadoutRects, image.Rectangle{})
	}
	dv.instEditorReadoutRects[idx] = r
}

// openSynthParamEditor opens the shared numeric editor anchored over the
// caption (readout) of knob at idx. The editor's setValue callback writes
// back through the knob + propagate path so every existing audio write stays
// consistent.
func (dv *DrumView) openSynthParamEditor(idx int, instID string) {
	if idx < 0 || idx >= len(dv.instEditorBindings) || idx >= len(dv.instEditorKnobs) {
		return
	}
	if dv.paramEditor == nil {
		dv.paramEditor = NewParamValueEditor()
	}
	def := dv.instEditorBindings[idx].def
	anchor := dv.synthKnobReadoutRect(idx)
	dv.paramEditor.OpenValue(ValueOpen{
		Spec:          paramSpec(def),
		Anchor:        anchor,
		MobileInputID: "synth-param",
		Get: func() float64 {
			return def.Min + dv.instEditorKnobs[idx].Value*(def.Max-def.Min)
		},
		Set: func(v float64) {
			span := def.Max - def.Min
			if span <= 0 {
				span = 1
			}
			dv.instEditorKnobs[idx].Value = (v - def.Min) / span
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
func (dv *DrumView) propagateSynthSliderValue(idx int, instID string) {
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
func (dv *DrumView) previewActiveSynth() {
	dv.auditionInstrumentAfterDrag(dv.synthTabActiveInstrument())
}

// auditionInstrumentAfterDrag fires one audition for an explicit user
// request. Its only production caller is previewActiveSynth (the Preview
// button) — knob releases and stage toggles are config-only and must not
// route here (see synthAuditionFn). instID is the already-resolved
// instrument id; the empty string is a no-op so callers don't have to gate
// on resolveSynthInstrument returning a value.
func (dv *DrumView) auditionInstrumentAfterDrag(instID string) {
	if instID == "" {
		return
	}
	synthAuditionFn(instID)
}

// resolveSynthInstrument resolves the editor's target row instrument id.
// Mirrors synthTabActiveInstrument but accepts a forced id (used by the
// callback bridge from EQPanelZone).
func (dv *DrumView) resolveSynthInstrument(instID string) string {
	if instID != "" {
		return instID
	}
	return dv.synthTabActiveInstrument()
}

// synthTabActiveInstrument resolves "which instrument's params is the
// synth tab editing right now?" Mirrors the channel-resolution logic in
// other audio tabs: prefer the EQ panel's active channel, fall back to
// the first audible/non-empty row.
func (dv *DrumView) synthTabActiveInstrument() string {
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
func (dv *DrumView) synthTabHitAreas() []HitArea {
	instID := dv.synthTabActiveInstrument()
	// Knobs sit two tiers above the EQ-panel chrome so the per-section scroll
	// catch-all (one tier above the chrome, below the knobs) lets a press that
	// lands on a knob still capture the knob, while a press on empty card space
	// scrolls. Pills / dialog / footer bump further so they win on overlap.
	scrollBodyZ := ZEQPanel + 1
	z := ZEQPanel + 2
	out := make([]HitArea, 0, len(dv.instEditorKnobs)+len(dv.instEditorBtns)+1)
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
	// Touch-min floor: expand the knob's hit ClipRect so the touchable area
	// is at least TouchMinTarget on each axis even when the knob's visual
	// rect is smaller (Compact-density knobs can be ≤ 28 px wide). Centring
	// the expanded rect on the visual rect keeps the press point honest —
	// the press latches against k.HandleInputResult, which still requires
	// the press to be inside the visual k.Rect for capture; the wider clip
	// only forgives near-miss taps that would otherwise fall through.
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
	for i, k := range dv.instEditorKnobs {
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
			Handler:  &synthKnobHitAdapter{dv: dv, idx: i, instID: instID},
			Tag:      fmt.Sprintf("synth-knob-%d", i),
			Touch:    true,
			ClipRect: clip,
		})
		// Also publish under the legacy "synth-slider-%d" tag so existing
		// browser tests / hit-area tests that target sliders still resolve.
		out = append(out, HitArea{
			Rect:     r,
			ZIndex:   z,
			Handler:  &synthKnobHitAdapter{dv: dv, idx: i, instID: instID},
			Tag:      fmt.Sprintf("synth-slider-%d", i),
			Touch:    true,
			ClipRect: clip,
		})
		// Readout (caption) hit area — sits below the dial, outside k.Rect().
		// OnPress checks for readout/badge before starting a drag, so tapping
		// here opens the numeric editor rather than capturing a knob drag.
		if rr := dv.synthKnobReadoutRect(i); !rr.Empty() {
			out = append(out, HitArea{
				Rect:     rr,
				ZIndex:   z,
				Handler:  &synthKnobHitAdapter{dv: dv, idx: i, instID: instID},
				Tag:      fmt.Sprintf("synth-readout-%d", i),
				Touch:    true,
				ClipRect: clipFor(rr),
			})
		}
		// Badge hit area — sits below the caption, outside k.Rect().
		// OnPress handles the cycle; this HitArea makes the badge reachable
		// via the real tree dispatch path.
		if i < len(dv.instEditorStepBadges) {
			if b := dv.instEditorStepBadges[i]; b != nil && !b.Rect().Empty() {
				out = append(out, HitArea{
					Rect:     b.Rect(),
					ZIndex:   z + 1, // above readout so badge wins over readout on overlap
					Handler:  &synthKnobHitAdapter{dv: dv, idx: i, instID: instID},
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
	if sel := dv.synthSelectedSection(); sel != nil && sel.enableParam != "" {
		if pillR := dv.synthDetailEnablePillRect(); !pillR.Empty() {
			out = append(out, HitArea{
				Rect:     pillR,
				ZIndex:   z + 1,
				Handler:  &synthToggleHitAdapter{dv: dv, param: sel.enableParam, instID: instID},
				Tag:      "synth-toggle-" + sel.enableParam,
				Touch:    true,
				ClipRect: clipFor(pillR),
			})
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
		// Route each footer button to its handler by sentinel text. Reset
		// is the legacy default; Save / Save-As were added in Phase 4.
		var handler HitHandler
		var tag string
		switch btn.Text {
		case synthSaveButtonTag:
			handler = &synthSaveHitAdapter{dv: dv}
			tag = "synth-save"
		case synthSaveAsButtonTag:
			handler = &synthSaveAsHitAdapter{dv: dv}
			tag = "synth-save-as"
		case synthPreviewButtonTag:
			handler = &synthPreviewHitAdapter{dv: dv}
			tag = "synth-preview"
		default: // synthResetButtonTag
			handler = &synthResetHitAdapter{dv: dv}
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
			Handler: handler,
			Tag:     tag,
			Touch:   true,
		})
	}
	return out
}

// synthSaveHitAdapter persists the active instrument's current params
// as the new defaults for its bound recipe (in-place override). The
// adapter is created fresh per Layout — see synthKnobHitAdapter for the
// rationale (avoids dangling pointers across re-layouts).
type synthSaveHitAdapter struct {
	dv *DrumView
}

func (h *synthSaveHitAdapter) OnPress(x, y int) InputResult {
	if h.dv != nil {
		h.dv.SaveActiveRecipe()
	}
	return InputCaptured
}

func (h *synthSaveHitAdapter) OnDrag(x, y int)                     {}
func (h *synthSaveHitAdapter) OnRelease(x, y int)                  {}
func (h *synthSaveHitAdapter) OnWheel(x, y, steps int) InputResult { return InputIgnored }

// synthSaveAsHitAdapter clones the active recipe into a new user-scoped
// recipe id and persists it. Phase 4 MVP semantics: the current row is
// not rebound (the new recipe shows up in the library for later
// selection); see SaveActiveRecipeAs in recipe_save_sink.go.
type synthSaveAsHitAdapter struct {
	dv *DrumView
}

func (h *synthSaveAsHitAdapter) OnPress(x, y int) InputResult {
	if h.dv != nil {
		h.dv.openSaveAsDialog()
	}
	return InputCaptured
}

func (h *synthSaveAsHitAdapter) OnDrag(x, y int)                     {}
func (h *synthSaveAsHitAdapter) OnRelease(x, y int)                  {}
func (h *synthSaveAsHitAdapter) OnWheel(x, y, steps int) InputResult { return InputIgnored }

// synthPreviewHitAdapter plays a one-shot audition of the active
// instrument's current (unsaved) tone. One-shot like the other header
// adapters; created fresh per Layout.
type synthPreviewHitAdapter struct {
	dv *DrumView
}

func (h *synthPreviewHitAdapter) OnPress(x, y int) InputResult {
	if h.dv != nil {
		h.dv.previewActiveSynth()
	}
	return InputCaptured
}

func (h *synthPreviewHitAdapter) OnDrag(x, y int)                     {}
func (h *synthPreviewHitAdapter) OnRelease(x, y int)                  {}
func (h *synthPreviewHitAdapter) OnWheel(x, y, steps int) InputResult { return InputIgnored }

// synthToggleHitAdapter routes a per-stage enable-pill tap to toggleSynthStage.
// One-shot like the footer Save/Reset adapters (no drag); created fresh per
// Layout so it never caches a stale pointer.
type synthToggleHitAdapter struct {
	dv     *DrumView
	param  string
	instID string
}

func (h *synthToggleHitAdapter) OnPress(x, y int) InputResult {
	if h.dv != nil {
		h.dv.toggleSynthStage(h.instID, h.param)
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

func (h *synthChipHitAdapter) OnPress(x, y int) InputResult {
	if h.dv == nil {
		return InputIgnored
	}
	// Ignore chip taps while a knob drag is captured: a selection swap would
	// zero the dragged knob's rect mid-gesture (same philosophy as
	// buildSynthTab's mid-drag schema-swap deferral). Consume so the tap
	// doesn't fall through to a lower-z surface either.
	if h.dv.anySynthKnobCapturing() {
		return InputConsumed
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

// synthKnobHitAdapter wraps a per-knob hit area with the synth-tab
// propagate callback. The adapter does NOT cache the *Knob pointer —
// buildSynthTab can rebuild instEditorKnobs mid-drag (analyzer state
// changes, channel switches, …), which would leave a cached pointer
// dangling against a now-orphan widget. Looking up by index every call
// stays correct across re-layouts. The captured pressY / pressVal stay
// inside the live Knob, so a re-layout preserves drag state because the
// new knob slot inherits the same Value from audio.MergeRecipeDefaults.
//
// The fresh-instance lookup also fixes a long-standing knob-drag bug:
// before, the captured *Knob would receive HandleInputResult calls that
// updated its own Value, but the visible widget (instEditorKnobs[idx])
// was a different object whose Value never changed — making the drag
// look unresponsive.
// Knob drag gesture modes. A press on a knob captures, then the first
// significant motion locks the axis: horizontal → adjust the knob (knobs are
// horizontal-only); vertical → hand the gesture to the knob's section scroll so
// up/down over a knob scrolls the list instead of fighting it.
const (
	knobDragUndecided = iota
	knobDragKnob
	knobDragScroll
)

// knobDragDirLockPx is how far the pointer must move from the press point before
// the knob/scroll axis is decided. Small so the lock feels immediate; vertical
// only wins when it strictly dominates, biasing ambiguous moves toward the knob.
const knobDragDirLockPx = 6

type synthKnobHitAdapter struct {
	dv     *DrumView
	idx    int
	instID string
	active bool
	mode   int
	pressX int
	pressY int
	// capturedParam is the binding param name at the instant the drag was
	// captured. The tree routes continued OnDrag to this captured-by-index
	// handler without re-testing hit areas, so if buildSynthTab ever rebuilt
	// the binding slice mid-gesture (e.g. a bespoke→modular schema swap when
	// the generator knob crosses Native→waveform) the index would point at a
	// DIFFERENT param and the drag would scramble it — silencing the re-voiced
	// voice when the foreign param is a gain/enable. buildSynthTab's deferral
	// (see anySynthKnobCapturing) prevents that swap; this name latch is the
	// defense-in-depth backstop: propagateIfStable refuses to drive a param
	// whose name drifted from capturedParam. Unit guard:
	// TestSynthKnobAdapter_PropagateIsIndexSafe.
	capturedParam string
}

// propagateIfStable pushes the captured knob's value to audio ONLY while the
// binding at this index still names the param the drag captured. If the synth
// schema rebuilt under an in-flight capture-by-index drag, the index now binds
// a foreign param; writing it would scramble (and possibly silence) the voice,
// so this no-ops instead. This is the second, independent layer behind the
// buildSynthTab schema-swap deferral — either alone prevents the bug.
func (h *synthKnobHitAdapter) propagateIfStable() {
	if h.dv == nil {
		return
	}
	nameNow := ""
	if h.idx >= 0 && h.idx < len(h.dv.instEditorBindings) {
		nameNow = h.dv.instEditorBindings[h.idx].def.Name
	}
	if h.capturedParam != "" {
		if h.idx < 0 || h.idx >= len(h.dv.instEditorBindings) || nameNow != h.capturedParam {
			h.dv.sdbg("[synthdrag] propagate BLOCKED idx=%d captured=%q now=%q — binding drifted under a captured-by-index drag (would have scrambled a foreign param)", h.idx, h.capturedParam, nameNow)
			return
		}
	}
	h.dv.syncSliderFromKnob(h.idx)
	h.dv.propagateSynthSliderValue(h.idx, h.instID)
}

// captureParamName latches the param name bound at this index at drag start so
// propagateIfStable can detect a mid-gesture binding drift.
func (h *synthKnobHitAdapter) captureParamName() {
	h.capturedParam = ""
	if h.dv != nil && h.idx >= 0 && h.idx < len(h.dv.instEditorBindings) {
		h.capturedParam = h.dv.instEditorBindings[h.idx].def.Name
	}
}

func (h *synthKnobHitAdapter) currentKnob() *Knob {
	if h.dv == nil || h.idx < 0 || h.idx >= len(h.dv.instEditorKnobs) {
		return nil
	}
	return h.dv.instEditorKnobs[h.idx]
}

// sectionGridForKnob returns the scrollable ControlGrid of the section that
// owns this knob, or nil when the section does not scroll.
func (h *synthKnobHitAdapter) sectionGridForKnob() *ControlGrid {
	if h.dv == nil {
		return nil
	}
	for _, s := range h.dv.instEditorSections {
		for _, idx := range s.knobIdxs {
			if idx == h.idx {
				if g := h.dv.sectionGrid(s.id); g != nil && g.HasScroll() {
					return g
				}
				return nil
			}
		}
	}
	return nil
}

func (h *synthKnobHitAdapter) invalidatePanel() {
	if h.dv != nil && h.dv.eqPanelZone != nil {
		h.dv.eqPanelZone.Invalidate()
	}
}

func (h *synthKnobHitAdapter) OnPress(x, y int) InputResult {
	// Numeric editor open: swallow the tap so a knob drag doesn't start beneath it.
	if h.dv.paramEditor != nil && h.dv.paramEditor.Active() {
		return InputConsumed
	}
	// Step-badge pill (lives inside the knob's hit rect, in the caption band):
	// a tap cycles the resolution instead of dragging.
	if h.idx < len(h.dv.instEditorStepBadges) {
		if b := h.dv.instEditorStepBadges[h.idx]; b != nil && !b.Rect().Empty() && image.Pt(x, y).In(b.Rect()) {
			h.dv.cycleKnobStep(h.idx)
			return InputConsumed
		}
	}
	// Value readout (caption line): a tap opens the numeric editor.
	if rr := h.dv.synthKnobReadoutRect(h.idx); !rr.Empty() && image.Pt(x, y).In(rr) {
		h.dv.openSynthParamEditor(h.idx, h.instID)
		return InputConsumed
	}
	k := h.currentKnob()
	if k == nil {
		return InputIgnored
	}
	result := k.HandleInputResult(x, y, true)
	if result != InputIgnored {
		h.active = true
		h.dv.setSynthSelectedKnob(h.instID, h.idx)
		h.mode = knobDragUndecided
		h.pressX, h.pressY = x, y
		h.captureParamName()
		// Stage 5: snapshot the pre-drag params for the concept-overlay ghost.
		h.dv.captureSynthGhost(h.idx, h.instID)
		h.dv.sdbg("[synthdrag] OnPress idx=%d capturedParam=%q inst=%s", h.idx, h.capturedParam, h.instID)
		h.propagateIfStable()
		return InputCaptured
	}
	return InputIgnored
}

func (h *synthKnobHitAdapter) OnDrag(x, y int) {
	if !h.active {
		return
	}
	if h.mode == knobDragUndecided {
		dx := x - h.pressX
		if dx < 0 {
			dx = -dx
		}
		dy := y - h.pressY
		if dy < 0 {
			dy = -dy
		}
		if dx < knobDragDirLockPx && dy < knobDragDirLockPx {
			return // wait for a clear direction before committing the axis
		}
		if dx > dy {
			// Horizontal strictly dominates → adjust the knob. Anything else
			// (vertical-dominant OR ties) is treated as a scroll, so a drag
			// that is exclusively or mostly up/down never turns the knob.
			h.mode = knobDragKnob
			h.dv.sdbg("[synthdrag] OnDrag axis-lock=KNOB idx=%d dx=%d dy=%d capturedParam=%q", h.idx, dx, dy, h.capturedParam)
		} else {
			// Vertical / diagonal: this drag is a scroll. Cancel the knob's
			// drag latch (its value is still the untouched press value) and
			// hand the gesture to the section's stepped scroll, anchored at
			// the press.
			h.mode = knobDragScroll
			h.dv.sdbg("[synthdrag] OnDrag axis-lock=SCROLL idx=%d dx=%d dy=%d (knob latch cancelled — param will NOT change)", h.idx, dx, dy)
			if k := h.currentKnob(); k != nil {
				k.HandleInputResult(x, y, false)
			}
			if g := h.sectionGridForKnob(); g != nil {
				g.BeginDrag(h.pressY)
			}
		}
	}
	switch h.mode {
	case knobDragScroll:
		if g := h.sectionGridForKnob(); g != nil && g.DragTo(y) {
			h.invalidatePanel()
		}
	case knobDragKnob:
		k := h.currentKnob()
		if k == nil {
			h.active = false
			return
		}
		k.HandleInputResult(x, y, true)
		h.propagateIfStable()
	}
}

func (h *synthKnobHitAdapter) OnRelease(x, y int) {
	if !h.active {
		return
	}
	if h.mode == knobDragScroll {
		if g := h.sectionGridForKnob(); g != nil {
			g.EndDrag()
		}
		h.active = false
		h.mode = knobDragUndecided
		return
	}
	k := h.currentKnob()
	if k != nil {
		k.HandleInputResult(x, y, false)
		h.propagateIfStable()
		// One undo step per gesture, committed here on release (the per-frame
		// drag mutated live audio but recorded nothing).
		h.dv.commitInstrumentParams(h.instID)
		// Stage 4: re-render the real note for the right-pane mirror (debounced +
		// cached by params hash, off the UI goroutine). Identical params coalesce.
		h.dv.requestSynthMirror(h.instID)
		// Stage 5: begin the concept-overlay ghost fade for this knob.
		h.dv.fadeSynthGhost(h.idx)
	}
	h.active = false
	h.mode = knobDragUndecided
	h.dv.sdbg("[synthdrag] OnRelease idx=%d capturedParam=%q inst=%s (config-only; next trigger renders the new params)", h.idx, h.capturedParam, h.instID)
	h.capturedParam = ""
}

func (h *synthKnobHitAdapter) OnWheel(x, y, steps int) InputResult {
	// Scrolling while hovering the step-RESOLUTION badge shifts its rung
	// (scroll up = finer, scroll down = coarser). This is the ONLY wheel
	// behavior added — the knob itself is left untouched.
	if h.idx >= 0 && h.idx < len(h.dv.instEditorStepBadges) {
		if b := h.dv.instEditorStepBadges[h.idx]; b != nil && !b.Rect().Empty() && image.Pt(x, y).In(b.Rect()) {
			if h.dv.stepSynthKnobResolution(h.idx, steps) {
				h.invalidatePanel()
			}
			return InputConsumed
		}
	}
	// Anywhere else (the knob dial): ORIGINAL behavior — the wheel is a
	// vertical gesture that scrolls the knob's section, never turns the knob.
	g := h.sectionGridForKnob()
	if g == nil {
		return InputIgnored
	}
	if g.WheelStep(steps) {
		h.invalidatePanel()
	}
	return InputConsumed
}

// OnWheel2D handles a two-finger trackpad drag with the raw axes. Over the
// step-resolution badge the scroll shifts the rung. Over the knob body a
// LEFT/RIGHT (horizontal-dominant) scroll changes the value — mirroring a
// left/right mouse drag — while an UP/DOWN scroll scrolls the section's
// overflow rows and never turns the knob.
func (h *synthKnobHitAdapter) OnWheel2D(x, y, dx, dy int) InputResult {
	// Resolution badge: either axis shifts the rung (prefer the vertical
	// component, matching the hover-the-label gesture).
	if h.idx >= 0 && h.idx < len(h.dv.instEditorStepBadges) {
		if b := h.dv.instEditorStepBadges[h.idx]; b != nil && !b.Rect().Empty() && image.Pt(x, y).In(b.Rect()) {
			steps := dy
			if steps == 0 {
				steps = dx
			}
			if h.dv.stepSynthKnobResolution(h.idx, steps) {
				h.invalidatePanel()
			}
			return InputConsumed
		}
	}
	// Horizontal-dominant: turn the knob (value), like a left/right mouse drag.
	// Debounced + magnitude-insensitive so it moves slowly; the per-step amount
	// is StepMul (the user's chosen resolution).
	if wheelAxisHorizontal(dx, dy) {
		if k := h.currentKnob(); k != nil && k.StepValueByWheel(dx) {
			h.dv.syncSliderFromKnob(h.idx)
			h.dv.propagateSynthSliderValue(h.idx, h.instID)
		}
		return InputConsumed
	}
	// Vertical (or no horizontal component): scroll the overflow rows.
	g := h.sectionGridForKnob()
	if g == nil {
		return InputIgnored
	}
	if g.WheelStep(dy) {
		h.invalidatePanel()
	}
	return InputConsumed
}

// wheelAxisHorizontal reports whether a two-axis wheel delta is horizontal-
// dominant (|dx| > |dy|). Ties go to vertical (false), matching the knob-drag
// axis lock, so a knob value only changes on a clearly sideways gesture.
func wheelAxisHorizontal(dx, dy int) bool {
	ax, ay := dx, dy
	if ax < 0 {
		ax = -ax
	}
	if ay < 0 {
		ay = -ay
	}
	return ax > ay
}

// synthSectionScrollAdapter routes scroll input for one overflowing section
// card to that section's ControlGrid. With body == true it is the card-body
// catch-all (a grab on empty card space); with body == false it is the
// scrollbar thumb handle. BOTH use the same row-by-row stepped drag
// (ControlGrid.BeginDrag/DragTo) — there is no continuous/momentum path — so
// the scroll feel is identical and deliberate wherever the user grabs. A scroll
// change calls Invalidate so the next Layout re-derives the knob rects for the
// new window. Created fresh per Layout (like synthKnobHitAdapter) so it never
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
	// Both thumb and body grabs start the same row-by-row stepped drag,
	// anchored at the press point. No continuous shift, no momentum fling.
	if g.BeginDrag(y) {
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
type synthSliderHitAdapter = synthKnobHitAdapter

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

// synthResetHitAdapter restores the active instrument's recipe to its shipped
// defaults (undoing any Save) and clears the per-instrument overlay when the
// reset button is clicked. Mirrors synthSaveHitAdapter — both route through the
// DrumView entry point so the active-instrument resolution stays in one place.
type synthResetHitAdapter struct {
	dv *DrumView
}

func (h *synthResetHitAdapter) OnPress(x, y int) InputResult {
	if h.dv != nil {
		h.dv.ResetActiveRecipe()
	}
	return InputCaptured
}

func (h *synthResetHitAdapter) OnDrag(x, y int)                     {}
func (h *synthResetHitAdapter) OnRelease(x, y int)                  {}
func (h *synthResetHitAdapter) OnWheel(x, y, steps int) InputResult { return InputIgnored }

// ---- Test accessors ----------------------------------------------------

// SynthTabSliders exposes the slider value-store for tests.
func (dv *DrumView) SynthTabSliders() []*Slider { return dv.instEditorSliders }

// SynthTabKnobs exposes the visible Knob widgets for tests.
func (dv *DrumView) SynthTabKnobs() []*Knob { return dv.instEditorKnobs }

// SynthTabBindings exposes the widget→param map for tests.
func (dv *DrumView) SynthTabBindings() []instParamBinding { return dv.instEditorBindings }

// SynthTabButtons exposes the editor buttons (currently just reset).
func (dv *DrumView) SynthTabButtons() []*Button { return dv.instEditorBtns }

// SynthTabSections exposes the section card layout for tests.
func (dv *DrumView) SynthTabSections() []synthSection { return dv.instEditorSections }

// SynthTabChips returns the pipeline chip strip model (tests, scenes, JS
// bridge). One entry per stage, in audio-pipeline order.
func (dv *DrumView) SynthTabChips() []synthChip { return dv.instEditorChips }

// SelectSynthSectionByLabel opens the stage whose chip label matches
// (case-insensitive, e.g. "OSC", "ENVELOPE"). Routes through the same
// selection path as a chip tap (minus the ghost-enable side effect) so the
// JS bridge and scene catalog drive exactly what a user click drives.
// Returns false when no chip carries the label.
func (dv *DrumView) SelectSynthSectionByLabel(label string) bool {
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
func (dv *DrumView) SynthSectionGrid(id synthSectionID) *ControlGrid {
	if dv.instEditorSectionGrids == nil {
		return nil
	}
	return dv.instEditorSectionGrids[id]
}

// SynthTabNoSynth reports whether the synth tab is showing its
// "instrument does not use the synth" banner (active row plays a WAV
// sample with no synth recipe). Exposed for tests.
func (dv *DrumView) SynthTabNoSynth() bool { return dv.instEditorNoSynth }

// SynthTabBannerRect exposes the no-synth banner geometry for tests.
func (dv *DrumView) SynthTabBannerRect() image.Rectangle { return dv.instEditorBannerRect }

// SynthTabHeaderButtons exposes the header retrigger / vol buttons so
// shared-widget discipline tests can walk them.
func (dv *DrumView) SynthTabHeaderButtons() []*Button { return dv.instEditorHeaderBtns }

// SynthSaveAsDialog returns the active Save As name dialog, or nil when
// none is open.
func (dv *DrumView) SynthSaveAsDialog() *synthSaveAsDialog { return dv.saveAsDialog }

// synthSaveDirty reports whether the active instrument has unsaved param
// changes relative to its bound recipe. Drives the Save button's spec
// switch (Primary when dirty, Secondary when clean) and the dialog's
// suggested default name.
func (dv *DrumView) synthSaveDirty(instID string) bool {
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
func (dv *DrumView) SynthTabHeader() synthHeaderLayout { return dv.instEditorHeader }

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

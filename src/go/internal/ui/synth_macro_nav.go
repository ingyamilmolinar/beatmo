package ui

import "github.com/ingyamilmolinar/beatmo/internal/instrument"

// synth_macro_nav.go owns the drill-down navigation state for the Synth tab's
// category layer: what (if anything) the user has drilled into. The root shows
// the category cards + the pipeline strip; a drilled target is either a
// CATEGORY (its params) or a pipeline STAGE (that stage's params) — both open
// the same drilled screen. State lives on DrumView (instEditorMacroOpen),
// keyed by resolved instID, plus the per-target "More" expander state.

// synthDrillKind says what kind of thing is drilled into.
type synthDrillKind int

const (
	drillNone     synthDrillKind = iota // the root
	drillCategory                       // a concept card (cat)
	drillStage                          // a pipeline stage (stage)
)

// synthDrillTarget is the drilled destination for one instrument.
type synthDrillTarget struct {
	kind  synthDrillKind
	cat   string
	stage synthSectionID
}

// key returns a stable map key for per-target state (the More expander).
func (t synthDrillTarget) key() string {
	switch t.kind {
	case drillCategory:
		return "cat:" + t.cat
	case drillStage:
		return "stage:" + sectionLabel(t.stage)
	}
	return ""
}

// synthDrillTarget returns the drilled destination for instID (kind drillNone
// at the root).
func (dv *DrumView) synthDrillTarget(instID instrument.ID) synthDrillTarget {
	if dv.instEditorMacroOpen == nil {
		return synthDrillTarget{}
	}
	return dv.instEditorMacroOpen[instID]
}

// synthMacroOpen returns the currently-drilled category id for instID ("" when
// at the root or drilled into a stage).
func (dv *DrumView) synthMacroOpen(instID instrument.ID) string {
	t := dv.synthDrillTarget(instID)
	if t.kind == drillCategory {
		return t.cat
	}
	return ""
}

// setSynthMacroOpen drills into category id (or "" to return to the root).
func (dv *DrumView) setSynthMacroOpen(instID instrument.ID, id string) {
	if id == "" {
		dv.setSynthDrillTarget(instID, synthDrillTarget{})
		return
	}
	dv.setSynthDrillTarget(instID, synthDrillTarget{kind: drillCategory, cat: id})
}

// setSynthStageOpen drills into pipeline stage id.
func (dv *DrumView) setSynthStageOpen(instID instrument.ID, id synthSectionID) {
	dv.setSynthDrillTarget(instID, synthDrillTarget{kind: drillStage, stage: id})
}

// The production UI has no drilled screens (the raw knobs are developer
// tools, devtools.go): every non-root target is refused here, the chokepoint
// the card tap, the pipeline pills, selectSynthSection and ui-state restore
// all reach, so the root stays open.
func (dv *DrumView) setSynthDrillTarget(instID instrument.ID, t synthDrillTarget) {
	if t.kind != drillNone && !devToolsOn() {
		return
	}
	if dv.instEditorMacroOpen == nil {
		dv.instEditorMacroOpen = map[instrument.ID]synthDrillTarget{}
	}
	dv.instEditorMacroOpen[instID] = t
}

// synthAtMacroRoot reports whether instID is at the root (nothing drilled in) —
// the landing view of category cards + the pipeline strip.
func (dv *DrumView) synthAtMacroRoot(instID instrument.ID) bool {
	return dv.synthDrillTarget(instID).kind == drillNone
}

// synthMoreOpen reports whether the drilled target's "More" (fine-tuning)
// params are expanded for instID. Session-only, per instrument + target.
func (dv *DrumView) synthMoreOpen(instID instrument.ID) bool {
	if dv.instEditorMoreOpen == nil {
		return false
	}
	return dv.instEditorMoreOpen[string(instID)+"|"+dv.synthDrillTarget(instID).key()]
}

// setSynthMoreOpen expands/collapses the drilled target's More section.
func (dv *DrumView) setSynthMoreOpen(instID instrument.ID, v bool) {
	if dv.instEditorMoreOpen == nil {
		dv.instEditorMoreOpen = map[string]bool{}
	}
	dv.instEditorMoreOpen[string(instID)+"|"+dv.synthDrillTarget(instID).key()] = v
}

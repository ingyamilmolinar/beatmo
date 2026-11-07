package ui

import "github.com/ingyamilmolinar/beatmo/internal/instrument"

// A drilled target is a CATEGORY or a pipeline STAGE, both opening the same screen; keyed by resolved instID.

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

func (dv *DrumView) synthDrillTarget(instID instrument.ID) synthDrillTarget {
	if dv.instEditorMacroOpen == nil {
		return synthDrillTarget{}
	}
	return dv.instEditorMacroOpen[instID]
}

// "" at the root or when drilled into a stage.
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

// Production has no drilled screens; every non-root target is refused here, the chokepoint every route reaches.
func (dv *DrumView) setSynthDrillTarget(instID instrument.ID, t synthDrillTarget) {
	if t.kind != drillNone && !devToolsOn() {
		return
	}
	if dv.instEditorMacroOpen == nil {
		dv.instEditorMacroOpen = map[instrument.ID]synthDrillTarget{}
	}
	dv.instEditorMacroOpen[instID] = t
}

func (dv *DrumView) synthAtMacroRoot(instID instrument.ID) bool {
	return dv.synthDrillTarget(instID).kind == drillNone
}

// Session-only, per instrument + target.
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

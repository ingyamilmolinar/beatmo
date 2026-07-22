package ui

// synth_macro_nav.go owns the drill-down navigation state for the beginner macro
// layer: which concept (if any) the user has drilled into. "" == the Basic root
// (concept cards shown); a concept id == that concept's params shown, with a
// "Basic" button to climb back. State lives on DrumView (instEditorMacroOpen),
// keyed by resolved instID, mirroring instEditorSelectedSection.

// synthMacroOpen returns the currently-drilled concept id for instID ("" = root).
func (dv *DrumView) synthMacroOpen(instID string) string {
	if dv.instEditorMacroOpen == nil {
		return ""
	}
	return dv.instEditorMacroOpen[instID]
}

// setSynthMacroOpen drills into concept id (or "" to return to the Basic root).
func (dv *DrumView) setSynthMacroOpen(instID, id string) {
	if dv.instEditorMacroOpen == nil {
		dv.instEditorMacroOpen = map[string]string{}
	}
	dv.instEditorMacroOpen[instID] = id
}

// synthAtMacroRoot reports whether instID is at the Basic root (no concept drilled
// in) — the beginner landing view of concept cards.
func (dv *DrumView) synthAtMacroRoot(instID string) bool {
	return dv.synthMacroOpen(instID) == ""
}

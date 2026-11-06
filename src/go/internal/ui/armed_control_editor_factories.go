package ui

import "github.com/ingyamilmolinar/beatmo/internal/instrument"

// armed_control_editor_factories.go — one factory per panel that hosts armed
// inline barrel controls. Each supplies the panel's two bindings plus the
// live-propagate + debounced-commit closure; ArmedControlEditor itself stays
// panel-agnostic (see synth_armed_editor.go).

// newSynthArmedControlEditor binds the armed editor to synth control idx of
// instID. The resolution lane is present only for params that carry a step
// badge (continuous params) — a discrete param gets a value lane only.
func newSynthArmedControlEditor(dv *DrumView, instID instrument.ID, idx int) *ArmedControlEditor {
	var resB barrelBinding
	if rb := (synthResolutionBinding{dv: dv, idx: idx, instID: instID}); rb.available() {
		resB = rb
	}
	return newArmedControlEditor(instID, idx,
		synthValueBinding{dv: dv, idx: idx, instID: instID}, resB,
		func() { dv.onSynthArmedEdit(instID, idx) })
}

// onSynthArmedEdit is the Synth tab's per-detent commit path: live slider sync +
// propagate now, ONE debounced undo step for the whole spin.
func (dv *DrumView) onSynthArmedEdit(instID instrument.ID, idx int) {
	dv.syncSliderFromKnob(idx)
	dv.propagateSynthSliderValue(idx, instID)
	dv.synthWheelNudge.bump(func() {
		dv.commitInstrumentParams(instID)
	})
	if dv.eqPanelZone != nil {
		dv.eqPanelZone.Invalidate()
	}
}

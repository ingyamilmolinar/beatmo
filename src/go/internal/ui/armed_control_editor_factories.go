package ui

import "github.com/ingyamilmolinar/beatmo/internal/instrument"

// One factory per host panel, so ArmedControlEditor itself stays panel-agnostic.

// A discrete param (no step badge) gets a value lane only.
func newSynthArmedControlEditor(dv *DrumView, instID instrument.ID, idx int) *ArmedControlEditor {
	var resB barrelBinding
	if rb := (synthResolutionBinding{dv: dv, idx: idx, instID: instID}); rb.available() {
		resB = rb
	}
	return newArmedControlEditor(instID, idx,
		synthValueBinding{dv: dv, idx: idx, instID: instID}, resB,
		func() { dv.onSynthArmedEdit(instID, idx) })
}

// Live sync per detent, ONE debounced undo step per spin.
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

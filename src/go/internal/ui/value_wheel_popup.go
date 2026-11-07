package ui

import (
	"image"
	"strconv"
)

// The shared wheel for non-knob values (BPM, node values): live per detent, ONE commit on close; Esc restores the
// opening value and commits nothing.

const valueWheelPortalID = "value-wheel-popup"

func (dv *DrumView) openValueWheel(b WheelBinding, anchor image.Rectangle) {
	dv.openValueWheelIn(b, anchor, dv.Bounds)
}

// For grid-pane openers (the sidebar), whose popup is then repainted above the pane divider.
func (dv *DrumView) openValueWheelIn(b WheelBinding, anchor, bounds image.Rectangle) {
	if dv.valueWheelPopup == nil {
		return
	}
	dv.CloseAllPopups()
	dv.valueWheelPopup.Open(b, anchor, bounds, dv.headerH)
	if dv.tree == nil {
		return
	}
	dv.portal().Open(PortalEntry{
		ID:      valueWheelPortalID,
		Overlay: &mobileWheelPopupPortalOverlay{popup: dv.valueWheelPopup, tag: valueWheelPortalID},
		Modal:   true,
		Scrim:   true,
		// Accept commits a pending edit and no-ops once closed.
		OnClose: func() { dv.valueWheelPopup.Accept() },
	})
}

// ValueWheelPopupRect is the open value wheel's rect (empty when closed).
func (dv *DrumView) ValueWheelPopupRect() image.Rectangle {
	if dv.valueWheelPopup == nil || !dv.valueWheelPopup.IsOpen() {
		return image.Rectangle{}
	}
	return dv.valueWheelPopup.Rect()
}

// bpmWheelLadder is the BPM wheel's step ladder (fine → coarse).
var bpmWheelLadder = []float64{1, 5, 10, 50}

// openBPMWheel opens the value wheel on the transport tempo.
func (dv *DrumView) openBPMWheel(anchor image.Rectangle) {
	z := dv.transportZone
	if z == nil {
		return
	}
	m := &NumberModel{
		Min: 1, Max: maxBPM, Value: float64(z.BPM()), Ladder: bpmWheelLadder,
		Format:       func(v, _ float64) string { return strconv.Itoa(int(v)) },
		LadderFormat: func(step float64) string { return "±" + strconv.Itoa(int(step)) },
	}
	dv.openValueWheel(WheelBinding{
		Model:    m,
		Title:    func() string { return "BPM" },
		Gloss:    func() string { return "tempo" },
		OnChange: func() { z.SetBPMLive(int(m.Value)) },
		OnCommit: func() { dv.commitBPM() },
	}, anchor)
}

// The live half; commitBPM announces. A tempo renders nothing: a voice is render.seconds at every tempo.
func (dv *DrumView) applyBPMState(bpm int) {
	dv.bpm = bpm
	dv.secPerBeat = 60.0 / float64(bpm)
	dv.logger.Debugf("[drumview] BPM set: -> %d", bpm)
}

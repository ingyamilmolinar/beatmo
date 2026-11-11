package ui

import (
	"image"
	"strconv"
)

// The shared value-wheel popup (dv.valueWheelPopup): a MobileWheelPopup over a
// NumberModel for values that are not synth knobs — the transport BPM and the
// node sidebar's volume / duration / logic / groove values. Every one of them
// applies LIVE on each detent and commits ONCE on close (Enter, tap-away,
// re-tapping the opener); Esc restores the opening value and commits nothing.

const valueWheelPortalID = "value-wheel-popup"

// openValueWheel opens the shared popup over anchor (inside the drum pane)
// with binding b.
func (dv *DrumView) openValueWheel(b WheelBinding, anchor image.Rectangle) {
	dv.openValueWheelIn(b, anchor, dv.Bounds)
}

// openValueWheelIn opens the shared popup over anchor, kept inside bounds —
// the window for openers in the grid pane (the node sidebar), whose popup is
// then repainted above the pane divider (Game.drawThemeOverlaysAboveDivider).
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
		// Persist-on-dismiss chokepoint: Accept commits a pending edit and
		// no-ops when the popup already closed (Enter / Esc).
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

// applyBPMState mirrors a tempo into DrumView (the live half of a BPM change;
// commitBPM / the ± path announce it). A tempo change renders nothing: a
// voice is render.seconds long at every tempo.
func (dv *DrumView) applyBPMState(bpm int) {
	dv.bpm = bpm
	dv.secPerBeat = 60.0 / float64(bpm)
	dv.logger.Debugf("[drumview] BPM set: -> %d", bpm)
}

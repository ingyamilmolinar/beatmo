package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

func (dv *DrumView) decayAnims() {
	decay := func(v *float64) {
		*v *= 0.85
		if *v < 0.01 {
			*v = 0
		}
	}
	// Transport animations are decayed by the zone's Update() when available.
	if dv.transportZone == nil {
		decay(&dv.playAnim)
		decay(&dv.stopAnim)
		decay(&dv.bpmDecAnim)
		decay(&dv.bpmIncAnim)
		decay(&dv.uploadAnim)
		decay(&dv.bpmErrorAnim)
	} else {
		// Bidirectional sync: BPM text input in DrumView.Update() may set
		// dv.bpmErrorAnim directly; propagate the higher value between
		// DrumView and zone so the toolbar cache sees it.
		if dv.bpmErrorAnim > dv.transportZone.bpmErrorAnim {
			dv.transportZone.bpmErrorAnim = dv.bpmErrorAnim
		} else {
			dv.bpmErrorAnim = dv.transportZone.bpmErrorAnim
		}
		decay(&dv.bpmErrorAnim)
		dv.transportZone.bpmErrorAnim = dv.bpmErrorAnim
	}
	// DrumView-specific animations.
	decay(&dv.lenDecAnim)
	decay(&dv.lenIncAnim)
	decay(&dv.saveAnim)
	// Reset delete confirmation after ~2s timeout
	if dv.deleteConfirmRow >= 0 && (dv.frame-dv.deleteConfirmFrame) >= 120 {
		dv.deleteConfirmRow = -1
		dv.markRowControlsDirty()
	}
}

func (dv *DrumView) renderToolbarControls(dst *ebiten.Image) {
	// Update only the rectangles/positions for existing per-row controls.
	dv.updateRowRects()
	// Delegate to TransportZone — it owns the toolbar cache.
	dv.transportZone.Draw(dst)
}

// drawNotifications renders up to 2 active notifications at the top-right of
// the drum view panel. Messages fade out as ttl decays.
func (dv *DrumView) drawNotifications(dst *ebiten.Image) {
	// Decay and prune
	out := dv.notifs[:0]
	for i := range dv.notifs {
		n := dv.notifs[i]
		if n.ttl <= 0 {
			continue
		}
		n.ttl--
		out = append(out, n)
	}
	dv.notifs = out
	// Draw last two (most recent at top)
	maxShow := 2
	pad := 6
	y := dv.Bounds.Min.Y + 6
	for i := len(dv.notifs) - 1; i >= 0 && maxShow > 0; i-- {
		n := dv.notifs[i]
		txt := n.msg
		w := TextWidth(txt)
		h := TextHeight() + pad
		boxW := w + pad*2
		x := dv.Bounds.Max.X - boxW - 10
		r := image.Rect(x, y, x+boxW, y+h)
		fill := colBPMBox
		if n.isErr {
			fill = colError
		}
		drawButton(dst, r, fill, colButtonBorder, false, Profile().DrawTopEdgeHighlight)
		DrawTextAt(dst, txt, r.Min.X+pad, r.Min.Y+(h-TextHeight())/2)
		y += h + 4
		maxShow--
	}
}

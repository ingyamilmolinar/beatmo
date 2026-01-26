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
	decay(&dv.playAnim)
	decay(&dv.stopAnim)
	decay(&dv.bpmDecAnim)
	decay(&dv.bpmIncAnim)
	decay(&dv.lenDecAnim)
	decay(&dv.lenIncAnim)
	decay(&dv.uploadAnim)
	decay(&dv.bpmErrorAnim)
	decay(&dv.saveAnim)
}

func (dv *DrumView) renderToolbarControls(dst *ebiten.Image) {
	// Update only the rectangles/positions for existing per-row controls.
	// Avoid recreating buttons and sliders every frame to keep rendering lightweight.
	dv.updateRowRects()

	dv.playBtn.Draw(dst)
	dv.stopBtn.Draw(dst)
	dv.bpmDecBtn.Draw(dst)
	dv.bpmBox.Draw(dst)
	if dv.bpmErrorAnim > 0 {
		drawRect(dst, dv.bpmBox.Rect, fadeColor(colError, dv.bpmErrorAnim), false)
	}
	dv.bpmIncBtn.Draw(dst)
	dv.subdivBtn.Draw(dst)
	dv.lenDecBtn.Draw(dst)
	dv.lenIncBtn.Draw(dst)
	dv.trackBtn.Draw(dst)
	dv.uploadBtn.Draw(dst)
	dv.importBtn.Draw(dst)
	dv.exportBtn.Draw(dst)
	if dv.mainVolSlider != nil {
		dv.mainVolSlider.Draw(dst)
	}
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
		w := debugCharW * len([]rune(txt))
		h := debugCharH + pad
		boxW := w + pad*2
		x := dv.Bounds.Max.X - boxW - 10
		r := image.Rect(x, y, x+boxW, y+h)
		fill := colBPMBox
		if n.isErr {
			fill = colError
		}
		// Slight transparency when close to expiry
		drawButton(dst, r, fill, colButtonBorder, false)
		// Right align text within the box with small inner pad
		DrawTextAt(dst, txt, r.Min.X+pad, r.Min.Y+(h-debugCharH)/2)
		y += h + 4
		maxShow--
	}
}

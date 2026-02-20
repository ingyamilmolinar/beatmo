package ui

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
)

// drawVolumeIconOffset draws a recognizable speaker icon in the row volume column.
// The icon scales to ~60% of cell height and shows volume-level arcs.
func (dv *DrumView) drawVolumeIconOffset(cache *ebiten.Image, rowIdx, offsetX, offsetY int) {
	if rowIdx >= len(dv.rowVolSliders) {
		return
	}
	r := dv.rowVolSliders[rowIdx].Rect()
	if r.Empty() {
		return
	}
	// Translate to cache coordinates.
	r = r.Sub(image.Pt(offsetX, offsetY))
	vol := 0.0
	if rowIdx < len(dv.Rows) {
		vol = dv.Rows[rowIdx].Volume
	}

	isMuted := rowIdx < len(dv.Rows) && dv.Rows[rowIdx].Muted

	// Icon color based on state.
	var iconCol color.RGBA
	if vol > 0 && !isMuted {
		iconCol = colVolumeIconOn
	} else {
		iconCol = colVolumeIconOff
	}

	// Scale icon to ~60% of cell height.
	cellH := r.Dy()
	iconH := cellH * 60 / 100
	if iconH < 8 {
		iconH = 8
	}
	iconW := iconH * 3 / 4 // aspect ratio
	if iconW < 6 {
		iconW = 6
	}

	cx := r.Min.X + r.Dx()/2
	cy := r.Min.Y + r.Dy()/2

	// Speaker body: left rectangle
	bodyW := iconW / 3
	if bodyW < 2 {
		bodyW = 2
	}
	bodyH := iconH / 2
	if bodyH < 4 {
		bodyH = 4
	}
	bodyLeft := cx - iconW/2
	bodyRect := image.Rect(bodyLeft, cy-bodyH/2, bodyLeft+bodyW, cy+bodyH/2)
	drawRect(cache, bodyRect, iconCol, true)

	// Cone (triangle extending right from body)
	coneRight := bodyLeft + iconW*2/3
	coneH := iconH * 3 / 4
	for dy := -coneH / 2; dy <= coneH/2; dy++ {
		t := 1.0 - float64(abs(dy))/float64(coneH/2+1)
		w := int(float64(coneRight-bodyRect.Max.X) * t)
		if w < 1 {
			w = 1
		}
		drawRect(cache, image.Rect(bodyRect.Max.X, cy+dy, bodyRect.Max.X+w, cy+dy+1), iconCol, true)
	}

	// Sound wave arcs (to the right of cone) based on volume level.
	if !isMuted && vol > 0 {
		arcX := coneRight + 2
		arcH := iconH / 2
		numArcs := 1
		if vol > 0.33 {
			numArcs = 2
		}
		if vol > 0.66 {
			numArcs = 3
		}
		for a := 0; a < numArcs; a++ {
			ax := arcX + a*3
			halfH := arcH/3 + a*arcH/4
			for dy := -halfH; dy <= halfH; dy++ {
				t := 1.0 - float64(abs(dy))/float64(halfH+1)
				if t > 0.4 {
					drawRect(cache, image.Rect(ax, cy+dy, ax+1, cy+dy+1), iconCol, true)
				}
			}
		}
	}
}

// drawMasterVolIconOffset draws a speaker icon for the master volume control.
// Uses mainVolSlider.Value for volume level, positioned at mainVolIconRect.
func (dv *DrumView) drawMasterVolIconOffset(cache *ebiten.Image, offsetX, offsetY int) {
	r := dv.mainVolIconRect
	if r.Empty() {
		return
	}
	r = r.Sub(image.Pt(offsetX, offsetY))
	vol := 0.0
	if dv.mainVolSlider != nil {
		vol = dv.mainVolSlider.Value
	}

	iconCol := colVolumeIconOn
	if vol <= 0 {
		iconCol = colVolumeIconOff
	}

	cellH := r.Dy()
	iconH := cellH * 60 / 100
	if iconH < 8 {
		iconH = 8
	}
	iconW := iconH * 3 / 4
	if iconW < 6 {
		iconW = 6
	}

	cx := r.Min.X + r.Dx()/2
	cy := r.Min.Y + r.Dy()/2

	bodyW := iconW / 3
	if bodyW < 2 {
		bodyW = 2
	}
	bodyH := iconH / 2
	if bodyH < 4 {
		bodyH = 4
	}
	bodyLeft := cx - iconW/2
	bodyRect := image.Rect(bodyLeft, cy-bodyH/2, bodyLeft+bodyW, cy+bodyH/2)
	drawRect(cache, bodyRect, iconCol, true)

	coneRight := bodyLeft + iconW*2/3
	coneH := iconH * 3 / 4
	for dy := -coneH / 2; dy <= coneH/2; dy++ {
		t := 1.0 - float64(abs(dy))/float64(coneH/2+1)
		w := int(float64(coneRight-bodyRect.Max.X) * t)
		if w < 1 {
			w = 1
		}
		drawRect(cache, image.Rect(bodyRect.Max.X, cy+dy, bodyRect.Max.X+w, cy+dy+1), iconCol, true)
	}

	if vol > 0 {
		arcX := coneRight + 2
		arcH := iconH / 2
		numArcs := 1
		if vol > 0.33 {
			numArcs = 2
		}
		if vol > 0.66 {
			numArcs = 3
		}
		for a := 0; a < numArcs; a++ {
			ax := arcX + a*3
			halfH := arcH/3 + a*arcH/4
			for dy := -halfH; dy <= halfH; dy++ {
				t := 1.0 - float64(abs(dy))/float64(halfH+1)
				if t > 0.4 {
					drawRect(cache, image.Rect(ax, cy+dy, ax+1, cy+dy+1), iconCol, true)
				}
			}
		}
	}
}

// openVolumePopup opens a vertical slider popup for the given row's volume.
func (dv *DrumView) openVolumePopup(rowIdx int) {
	if rowIdx < 0 || rowIdx >= len(dv.Rows) {
		return
	}
	dv.CloseAllPopups()

	iconRect := image.Rectangle{}
	if rowIdx < len(dv.rowVolSliders) {
		iconRect = dv.rowVolSliders[rowIdx].Rect()
	}
	if iconRect.Empty() {
		return
	}

	dv.volPopupRow = rowIdx
	dv.volPopup.Open(iconRect, dv.Bounds, dv.headerH)
}

func (dv *DrumView) closeVolumePopup() {
	dv.volPopup.Close()
}

// drawVolumePopup renders the volume popup panel.
func (dv *DrumView) drawVolumePopup(dst *ebiten.Image) {
	dv.volPopup.Draw(dst)
}

/* ─── master volume popup ─────────────────────────────────── */

// openMasterVolumePopup opens a vertical slider popup above the master volume icon.
func (dv *DrumView) openMasterVolumePopup() {
	dv.CloseAllPopups()
	iconRect := dv.mainVolIconRect
	if iconRect.Empty() {
		return
	}
	dv.masterVolPopup.Open(iconRect, dv.Bounds, dv.headerH)
}

func (dv *DrumView) closeMasterVolumePopup() {
	dv.masterVolPopup.Close()
}

// drawMasterVolumePopup renders the master volume popup panel.
func (dv *DrumView) drawMasterVolumePopup(dst *ebiten.Image) {
	dv.masterVolPopup.Draw(dst)
}

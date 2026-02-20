package ui

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
)

// eqCenterLabels are the ISO center frequency labels for the 10 EQ bands.
var eqCenterLabels = [10]string{"31", "62", "125", "250", "500", "1k", "2k", "4k", "8k", "16k"}

// drawEQBandButton draws a small button-like EQ level indicator with 3 horizontal
// bars of varying heights reflecting the current gain. The middle bar length
// reflects the band gain (shorter when cut, longer when boost).
func (dv *DrumView) drawEQBandButton(dst *ebiten.Image, bandIdx int, r image.Rectangle) {
	if r.Empty() {
		return
	}

	// Get current gain value as 0..1.
	val := 0.5 // default = 0 dB = center
	ch := dv.activeEQChannel()
	if ch == "main" {
		if bandIdx < len(dv.eqBandGainsDB) {
			val = gainDBToSlider(dv.eqBandGainsDB[bandIdx])
		}
	} else {
		for _, row := range dv.Rows {
			if row.Instrument == ch && bandIdx < len(row.EQGainsDB) {
				val = gainDBToSlider(row.EQGainsDB[bandIdx])
				break
			}
		}
	}

	// Button background.
	drawRect(dst, r, colStep, true)
	drawRect(dst, r, colSubtleBorder, false)

	// 3 horizontal bars at 25%, 50%, 75% height.
	cx := r.Min.X + r.Dx()/2
	maxBarW := r.Dx() - 6 // padding 3px each side
	if maxBarW < 2 {
		maxBarW = 2
	}
	barH := 1
	if r.Dy() > 10 {
		barH = 2
	}

	positions := [3]float64{0.25, 0.50, 0.75}
	for i, frac := range positions {
		by := r.Min.Y + int(float64(r.Dy())*frac) - barH/2
		barW := maxBarW
		if i == 1 {
			// Middle bar: length reflects gain (val 0..1 → 30%..100% of maxBarW).
			barW = maxBarW*30/100 + int(float64(maxBarW*70/100)*val)
			if barW < 2 {
				barW = 2
			}
		}
		bx := cx - barW/2
		drawRect(dst, image.Rect(bx, by, bx+barW, by+barH), color.RGBA{255, 255, 255, 180}, true)
	}
}

// openEQPopup opens a vertical slider popup for the given EQ band.
func (dv *DrumView) openEQPopup(bandIdx int) {
	if bandIdx < 0 || bandIdx >= len(eqBandDefs) {
		return
	}
	dv.CloseAllPopups()

	numBands := len(eqBandDefs)
	bandW := dv.eqRect.Dx() / numBands
	if bandW <= 0 {
		return
	}

	// Band column center X → build an anchor rect for positioning.
	bandCX := dv.eqRect.Min.X + bandIdx*bandW + bandW/2
	anchor := image.Rect(bandCX-bandW/2, dv.eqRect.Min.Y, bandCX+bandW/2, dv.eqRect.Max.Y)

	dv.eqPopupBand = bandIdx
	dv.eqPopup.Open(anchor, dv.Bounds, dv.headerH)
}

// closeEQPopup closes the EQ band popup.
func (dv *DrumView) closeEQPopup() {
	dv.eqPopup.Close()
}

// drawEQPopup renders the EQ band popup panel.
func (dv *DrumView) drawEQPopup(dst *ebiten.Image) {
	dv.eqPopup.Draw(dst)
}

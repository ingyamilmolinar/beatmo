package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

func (dv *DrumView) ensureHighlightSprites() {
	h := dv.rowHeight()
	if h <= 0 {
		h = 1
	}
	if dv.hlSpriteReg != nil && dv.hlSpriteH == h {
		return
	}
	// Base regular highlight: semi-transparent overlay
	reg := ebiten.NewImage(1, h)
	drawRect(reg, image.Rect(0, 0, 1, h), fadeColor(colHighlight, 0.5), true)
	// Mute highlight: use existing colMuteHighlight
	mute := ebiten.NewImage(1, h)
	drawRect(mute, image.Rect(0, 0, 1, h), colMuteHighlight, true)
	dv.hlSpriteReg = reg
	dv.hlSpriteMute = mute
	dv.hlSpriteH = h
}

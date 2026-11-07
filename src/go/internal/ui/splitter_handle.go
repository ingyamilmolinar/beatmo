package ui

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
)

// Shared by both dividers, so the glow+grow is pixel-identical on each.
type SplitterHandle struct {
	hoverAnim float64 // 0 = rest, 1 = fully hovered
}

// Settles to EXACTLY 1.0 within ~7 frames, well inside the ~12-frame windows the divider tests assert.
const splitterHoverStep = 1.0 / 6.0

// HoverAnim returns the current hover progress in [0, 1].
func (h *SplitterHandle) HoverAnim() float64 { return h.hoverAnim }

// Overshoot clamps to EXACTLY 1.0/0.0 so the line lands precisely on its theme token.
func (h *SplitterHandle) Advance(hover bool) {
	if hover {
		h.hoverAnim += splitterHoverStep
	} else {
		h.hoverAnim -= splitterHoverStep
	}
	if h.hoverAnim > 1 {
		h.hoverAnim = 1
	} else if h.hoverAnim < 0 {
		h.hoverAnim = 0
	}
}

// Shared with the EQ divider so both pills are pixel-identical.
func (h *SplitterHandle) DrawHorizontalDivider(dst *ebiten.Image, x0, x1, y int) {
	drawRect(dst, image.Rect(x0, y-1, x1, y), genColorDividerShadow, true)
	drawRect(dst, image.Rect(x0, y, x1, y+1), h.lineColor(), true)
	drawRect(dst, image.Rect(x0, y+1, x1, y+2), WithAlpha(colAccent, AlphaSubtle), true)
	// The pill straddles the line on both dividers, so neither floats off its horizon.
	h.drawHandle(dst, (x0+x1)/2, y, true)
}

func (h *SplitterHandle) DrawVerticalDivider(dst *ebiten.Image, y0, y1, x int) {
	drawRect(dst, image.Rect(x-1, y0, x, y1), genColorDividerShadow, true)
	drawRect(dst, image.Rect(x, y0, x+1, y1), h.lineColor(), true)
	drawRect(dst, image.Rect(x+1, y0, x+2, y1), WithAlpha(colAccent, AlphaSubtle), true)
	h.drawHandle(dst, x, (y0+y1)/2, false)
}

// Both ends are exact theme tokens, never an interpolated literal, so the tests' exact-colour match holds.
func (h *SplitterHandle) lineColor() color.RGBA {
	if h.hoverAnim >= 1 {
		return colAccentBright
	}
	return colAccent
}

// The halo is always larger than the static handle glow, so it is the rect the grow test measures.
func (h *SplitterHandle) drawHandle(dst *ebiten.Image, cx, cy int, horizontal bool) {
	p := h.hoverAnim
	base := SplitterHandleRect(cx, cy, horizontal)

	// A restrained grow: hover reads as a gentle lift, not a bloom.
	pad := 3 + int(p*3+0.5)
	halo := base.Inset(-pad)
	alpha := uint8(40 + p*90)
	drawRect(dst, halo, WithAlpha(colAccent, alpha), true)

	DrawSplitterHandle(dst, cx, cy, horizontal, p > 0.5)
}

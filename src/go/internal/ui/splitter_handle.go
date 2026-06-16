package ui

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
)

// SplitterHandle owns the shared animated line+pill draw used by both the
// main grid↔drum splitter (Splitter.handle) and the EQ-boundary divider
// (rowEQDividerLayer.handle). It encapsulates the hoverAnim easing so the
// same glow+grow behavior is pixel-identical on both dividers.
type SplitterHandle struct {
	hoverAnim float64 // 0 = rest, 1 = fully hovered
}

// splitterHoverStep is the per-frame ramp for the hover animation. 1/6 settles
// the pill to a clamped, EXACT 1.0 within ~7 frames (and back to 0.0 the same
// way), so the line reaches its bright variant and the glow its full size well
// inside the ~12-frame windows the divider tests assert.
const splitterHoverStep = 1.0 / 6.0

// HoverAnim returns the current hover progress in [0, 1].
func (h *SplitterHandle) HoverAnim() float64 { return h.hoverAnim }

// Advance ramps hoverAnim toward 1 while hovered and toward 0 otherwise, a
// fixed step per frame, clamped to [0,1]. Overshoot clamps to EXACTLY 1.0 (or
// 0.0) so the eased line color lands precisely on colAccentBright / colAccent.
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

// DrawHorizontalDivider draws the azure "horizon line" (shadow + accent +
// bloom) full width across [x0,x1) at y, with the animated pill centered. The
// accent line lifts to its bright variant on hover; the pill's glow grows and
// brightens. Mirrors (*Game).drawDivider and the EQ divider so both pills are
// pixel-identical.
func (h *SplitterHandle) DrawHorizontalDivider(dst *ebiten.Image, x0, x1, y int) {
	drawRect(dst, image.Rect(x0, y-1, x1, y), genColorDividerShadow, true)
	drawRect(dst, image.Rect(x0, y, x1, y+1), h.lineColor(), true)
	drawRect(dst, image.Rect(x0, y+1, x1, y+2), WithAlpha(colAccent, AlphaSubtle), true)
	h.drawHandle(dst, (x0+x1)/2, y, true)
}

// DrawVerticalDivider is the side-by-side (left/right) counterpart of
// DrawHorizontalDivider, spanning [y0,y1) at x.
func (h *SplitterHandle) DrawVerticalDivider(dst *ebiten.Image, y0, y1, x int) {
	drawRect(dst, image.Rect(x-1, y0, x, y1), genColorDividerShadow, true)
	drawRect(dst, image.Rect(x, y0, x+1, y1), h.lineColor(), true)
	drawRect(dst, image.Rect(x+1, y0, x+2, y1), WithAlpha(colAccent, AlphaSubtle), true)
	h.drawHandle(dst, x, (y0+y1)/2, false)
}

// lineColor is the accent horizon line: azure colAccent at rest, lifting to
// the brighter colAccentBright once the hover animation has fully settled.
// Both endpoints are the exact theme tokens (no interpolated literal), so the
// divider tests' exact-color match holds.
func (h *SplitterHandle) lineColor() color.RGBA {
	if h.hoverAnim >= 1 {
		return colAccentBright
	}
	return colAccent
}

// drawHandle paints the pill at (cx,cy) with a glow halo that grows in size
// and alpha as the hover animation advances. The halo is always larger than
// the static DrawSplitterHandle glow so it is the dominant rect the pill-grow
// test measures.
func (h *SplitterHandle) drawHandle(dst *ebiten.Image, cx, cy int, horizontal bool) {
	p := h.hoverAnim
	base := SplitterHandleRect(cx, cy, horizontal)

	// Glow halo: a restrained grow so the hover affordance reads as a gentle
	// lift, not a bloom. Pad 3px at rest → 6px at full hover; alpha 40 → 130.
	pad := 3 + int(p*3+0.5)
	halo := base.Inset(-pad)
	alpha := uint8(40 + p*90)
	drawRect(dst, halo, WithAlpha(colAccent, alpha), true)

	// Pill body (square handle + its own small static glow), brightening past
	// the animation midpoint.
	DrawSplitterHandle(dst, cx, cy, horizontal, p > 0.5)
}

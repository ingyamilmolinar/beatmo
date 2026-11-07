package ui

// Depth comes from TONE, feedback from MOTION, and the accent marks VALUE, never chrome; the focus ring is the
// only outline left.

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
)

// A hairline constant like the 1 px separator, not a sizing decision, so not a density token.
const flatSelectionBarPx = 2

// Horizontal groups mark the BOTTOM, vertical lists the LEFT: one primitive, two orientations.
type selectionEdge int

const (
	selectionEdgeBottom selectionEdge = iota
	selectionEdgeLeft
)

// One filled rect and nothing else. A transparent fill draws nothing, so `button-quiet` costs no draws at rest.
func drawFlatButtonBody(dst *ebiten.Image, r image.Rectangle, fill color.Color, rad int) {
	if r.Empty() || fill == nil {
		return
	}
	if _, _, _, a := fill.RGBA(); a == 0 {
		return
	}
	if rad <= 0 {
		drawRect(dst, r, fill, true)
		return
	}
	drawRoundedRect(dst, r, fill, rad, true)
}

// frac grows the bar from the centre (a hover spring); a left bar is always full height. frac <= 0 draws nothing.
func drawFlatSelectionBar(dst *ebiten.Image, r image.Rectangle, col color.Color, edge selectionEdge, frac float64) {
	if r.Empty() || col == nil || frac <= 0 {
		return
	}
	if frac > 1 {
		frac = 1
	}
	switch edge {
	case selectionEdgeLeft:
		drawRect(dst, image.Rect(r.Min.X, r.Min.Y, r.Min.X+flatSelectionBarPx, r.Max.Y), col, true)
	default: // selectionEdgeBottom
		w := int(float64(r.Dx()) * frac)
		if w < 1 {
			return
		}
		cx := (r.Min.X + r.Max.X) / 2
		x0 := cx - w/2
		drawRect(dst, image.Rect(x0, r.Max.Y-flatSelectionBarPx, x0+w, r.Max.Y), col, true)
	}
}

// The DESIGN.md deltas (hover +12, press -20), so the restyle changed geometry, not feel. Disabled uses
// surface-1 so a dead control reads as part of its container.
func flatStateFill(base color.Color, st ComponentState) color.Color {
	if st.Disabled {
		return colSurface1
	}
	switch {
	case st.Pressed:
		return adjustColor(base, -20)
	case st.Hovered:
		return adjustColor(base, 12)
	}
	return base
}

// Returns the full rect: there is no cap face distinct from the button.
func drawFlatPanelButton(dst *ebiten.Image, r image.Rectangle, fill color.Color, hovered bool) image.Rectangle {
	rad := fitRadius(popupButtonRadius(), r)
	if hovered {
		fill = adjustColor(fill, 12)
	}
	drawFlatButtonBody(dst, r, fill, rad)
	return r
}

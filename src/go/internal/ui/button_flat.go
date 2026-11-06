package ui

// button_flat.go — the flat-tonal button primitives that replace the retired
// matte-keycap chrome. See DESIGN.md "Elevation & Depth".
//
// Doctrine: depth comes from TONE (a control sits one surface level above its
// container), feedback comes from MOTION (hover/press deltas + a growing
// selection bar), and the accent color marks VALUE, never chrome. A control
// draws no border at rest — the only outline left in the system is the
// hot-pink focus ring.

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
)

// flatSelectionBarPx is the thickness of the accent selection bar. Two px
// reads at every density; it is a hairline constant like the 1px separator,
// not a sizing decision, so it is deliberately not a density token.
const flatSelectionBarPx = 2

// selectionEdge names which edge of a control carries its selection bar.
// Horizontal groups (tab strips, segmented controls) mark the BOTTOM; vertical
// lists (menus) mark the LEFT. One primitive, two orientations — see DESIGN.md
// "One selection primitive".
type selectionEdge int

const (
	selectionEdgeBottom selectionEdge = iota
	selectionEdgeLeft
)

// drawFlatButtonBody paints a control's body: a single filled rounded rect and
// nothing else. No socket wall, no contact shadow, no bevel, no border stroke.
// The control's silhouette is the tone step between its fill and its container.
//
// A fully transparent fill paints nothing, which is what lets the
// `button-quiet` variant cost zero draw calls at rest.
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

// drawFlatSelectionBar paints the accent selection bar along r's edge. frac
// (0..1) scales the bar's length so a hover spring can grow it from nothing to
// full; a bottom bar grows outward from the center, while a left bar is always
// full-height (a list row's stripe does not animate its length). frac <= 0
// draws nothing, which is what keeps an unselected control at zero draw cost.
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

// flatStateFill applies the interaction brightness deltas to a base fill.
// The deltas are the historical ButtonStyle values retained verbatim from
// DESIGN.md `button-secondary` (hover +12, press -20), so the restyle changes
// a button's GEOMETRY, not its interaction feel. Disabled short-circuits to
// surface-1 so a dead control reads as part of its container rather than as a
// dimmed object sitting on it.
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

// drawFlatPanelButton renders a transient popup/panel action as a flat tonal
// chip and returns the content rect so callers can center a label on it.
// Replaces drawKeycapPanelButton; it returns the full rect because there is no
// longer a cap face distinct from the button rect.
func drawFlatPanelButton(dst *ebiten.Image, r image.Rectangle, fill color.Color, hovered bool) image.Rectangle {
	rad := fitRadius(popupButtonRadius(), r)
	if hovered {
		fill = adjustColor(fill, 12)
	}
	drawFlatButtonBody(dst, r, fill, rad)
	return r
}

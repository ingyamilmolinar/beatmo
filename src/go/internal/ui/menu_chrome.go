package ui

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
)

// menuItemState is the visual state of a menu row.
type menuItemState int

const (
	menuItemRest menuItemState = iota
	menuItemHover
	menuItemActive
)

// accentStripeW is the density-aware left accent-stripe width used by active
// menu items (shares the row-rack accent stripe token).
func accentStripeW() int {
	w := Profile().DensityValues().AccentStripeWidth
	if w < 2 {
		w = 2
	}
	return w
}

// drawMenuHeaderBand paints a header band with a faint sunset gradient
// (grid-horizon → surface-overlay) and a 1px azure underline — the shared
// "Neon Horizon" header for identity menus.
func drawMenuHeaderBand(dst *ebiten.Image, r image.Rectangle) {
	drawMenuHeaderBandAccent(dst, r, colAccent)
}

// drawMenuHeaderBandAccent is drawMenuHeaderBand with a caller-supplied accent
// color for the underline. Per-instrument menus pass the instrument color so
// the header reads as owned by the instrument; global menus pass colAccent.
func drawMenuHeaderBandAccent(dst *ebiten.Image, r image.Rectangle, accent color.Color) {
	if r.Empty() {
		return
	}
	fillVerticalGradient(dst, r, genColorGridHorizon, genColorSurfaceOverlay, 16)
	underline := image.Rect(r.Min.X, r.Max.Y-1, r.Max.X, r.Max.Y)
	drawRect(dst, underline, WithAlphaFromColor(accent, genAlphaMenuHeaderAccent), true)
}

// drawMenuItemBackground paints the per-state background of a menu row:
// rest = nothing; hover = cushion lift + faint azure glow; active = left
// azure stripe + tinted fill.
func drawMenuItemBackground(dst *ebiten.Image, r image.Rectangle, state menuItemState) {
	drawMenuItemBackgroundAccent(dst, r, state, colAccent)
}

// drawMenuItemBackgroundAccent is drawMenuItemBackground with a caller-supplied
// accent color for the active stripe + hover/active tints. Per-instrument menus
// pass the instrument color; global menus pass colAccent.
func drawMenuItemBackgroundAccent(dst *ebiten.Image, r image.Rectangle, state menuItemState, accent color.Color) {
	if r.Empty() {
		return
	}
	switch state {
	case menuItemHover:
		drawRect(dst, r, colSurface3, true)
		drawRect(dst, r, WithAlphaFromColor(accent, genAlphaSubtle), true)
	case menuItemActive:
		drawRect(dst, r, WithAlphaFromColor(accent, genAlphaMenuActiveTint), true)
		stripe := image.Rect(r.Min.X, r.Min.Y, r.Min.X+accentStripeW(), r.Max.Y)
		drawRect(dst, stripe, accent, true)
	}
}

// drawMenuChevron draws the icon-system chevron (down=expanded, right=collapsed)
// centered in r, tinted col.
func drawMenuChevron(dst *ebiten.Image, r image.Rectangle, expanded bool, col color.Color) {
	id := IconChevronRight
	if expanded {
		id = IconChevronDown
	}
	dim := IconSizeMD
	cx := r.Min.X + r.Dx()/2
	cy := r.Min.Y + r.Dy()/2
	iconR := image.Rect(cx-dim/2, cy-dim/2, cx+dim/2, cy+dim/2)
	DrawIcon(dst, id, iconR, col)
}

package ui

import "image"

// gridHelpReservedMargin is the breathing room added around the settings-gear
// button when reserving its protected zone: popups clear the gear by this much
// so their borders never touch it.
const gridHelpReservedMargin = SpaceSM

// gridHelpReservedRect is the protected "no-invade" zone around the settings
// gear in the grid pane's top-right corner: the gear button rect inflated by
// gridHelpReservedMargin on every side. No grid-pane popup may render its
// panel inside this rect. Empty (returned as-is) when the gear does not exist.
//
// Because the grid pane's origin is always screen (0,0) (Splitter.GridRect),
// this rect is directly comparable to every popup's grid-local geometry — no
// coordinate translation is needed at the call sites.
func (g *Game) gridHelpReservedRect() image.Rectangle {
	r := g.gridHelpButtonRect()
	if r.Empty() {
		return image.Rectangle{}
	}
	return r.Inset(-gridHelpReservedMargin)
}

// avoidReservedRect nudges panel out of reserved — a top-right-corner exclusion
// zone — while keeping it inside the grid pane [0,gridW]×[0,gridH]. It returns
// the adjusted panel:
//
//  1. reserved empty or no overlap → panel unchanged.
//  2. the panel fits below the zone → pushed straight down so its top sits at
//     reserved.Max.Y (the natural escape for the top-right corner).
//  3. otherwise, if it fits to the left → pushed left so its right edge sits at
//     reserved.Min.X.
//  4. otherwise (pane too small to avoid) → best-effort pane clamp; the gear
//     still paints on top by z-order.
//
// The push directions match the gear's fixed top-right position: escaping down
// or left always moves toward more open space.
func avoidReservedRect(panel, reserved image.Rectangle, gridW, gridH int) image.Rectangle {
	if reserved.Empty() || !panel.Overlaps(reserved) {
		return panel
	}

	w, h := panel.Dx(), panel.Dy()

	// Prefer pushing down (below the gear) when the panel still fits vertically.
	if reserved.Max.Y+h <= gridH {
		return clampRectToPane(image.Rect(panel.Min.X, reserved.Max.Y, panel.Min.X+w, reserved.Max.Y+h), gridW, gridH)
	}

	// Fall back to pushing left (clear of the gear column) when it fits.
	if reserved.Min.X-w >= 0 {
		return clampRectToPane(image.Rect(reserved.Min.X-w, panel.Min.Y, reserved.Min.X, panel.Min.Y+h), gridW, gridH)
	}

	// Pane too small to avoid the zone: best-effort clamp. The gear's top
	// z-order keeps it painted on top regardless.
	return clampRectToPane(panel, gridW, gridH)
}

// clampRectToPane shifts r (preserving its size) to lie within [0,gridW]×[0,gridH].
func clampRectToPane(r image.Rectangle, gridW, gridH int) image.Rectangle {
	w, h := r.Dx(), r.Dy()
	x, y := r.Min.X, r.Min.Y
	if x+w > gridW {
		x = gridW - w
	}
	if x < 0 {
		x = 0
	}
	if y+h > gridH {
		y = gridH - h
	}
	if y < 0 {
		y = 0
	}
	return image.Rect(x, y, x+w, y+h)
}

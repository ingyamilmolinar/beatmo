package ui

import "image"

// Popups clear the gear by this much so their borders never touch it.
const gridHelpReservedMargin = SpaceSM

// No grid-pane popup may render inside it. The grid pane's origin is screen (0,0), so it compares
// directly with popup geometry.
func (g *Game) gridHelpReservedRect() image.Rectangle {
	gear := g.gridHelpButtonRect()
	if gear.Empty() {
		return image.Rectangle{}
	}
	zone := gear
	if file := g.gridFileButtonRect(); !file.Empty() {
		zone = zone.Union(file) // protect the folder button too
	}
	return zone.Inset(-gridHelpReservedMargin)
}

// The WHOLE cluster: a popup's own close button must clear it, or a higher-z grid button swallows
// the click. gridHelpReservedRect stays narrower for the group menu's tiny-window placement.
func (g *Game) gridTopButtonsReservedRect() image.Rectangle {
	zone := g.gridHelpReservedRect() // gear+file, already inset
	if logs := g.gridLogsButtonRect(); !logs.Empty() {
		lz := logs.Inset(-gridHelpReservedMargin)
		if zone.Empty() {
			zone = lz
		} else {
			zone = zone.Union(lz)
		}
	}
	if th := g.gridThemeButtonRect(); !th.Empty() {
		tz := th.Inset(-gridHelpReservedMargin)
		if zone.Empty() {
			zone = tz
		} else {
			zone = zone.Union(tz)
		}
	}
	return zone
}

// Nothing is created here: a near miss must be inert. Uses the cluster handler's own bounds, so
// "a button took the press" and "the grid may act" never disagree.
func (g *Game) inTopRightButtonZone(x, y int) bool {
	zone := g.gridTopButtonsInputRect()
	if zone.Empty() {
		return false
	}
	return image.Pt(x, y).In(zone)
}

// No overlap: unchanged. Else push down below the zone, else left of it, else clamp to the pane
// (the gear still paints on top). Down and left always lead toward open space.
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

	// The gear's z keeps it on top regardless.
	return clampRectToPane(panel, gridW, gridH)
}

// Candidates adjacent to the ANCHOR (right, left, below, above), each clamped and gear-adjusted,
// so the menu always stays next to what opened it. The first whose final rect overlaps NO member wins
// (per member, not a union bbox); otherwise the least total overlap.
func placeGroupMenu(anchor image.Point, size image.Point, members []image.Rectangle, gearZone image.Rectangle, gridW, gridH int) image.Rectangle {
	w, h := size.X, size.Y
	ax, ay := anchor.X, anchor.Y
	candidates := [4]image.Rectangle{
		image.Rect(ax, ay, ax+w, ay+h),       // right of anchor (top-left at anchor)
		image.Rect(ax-w, ay, ax, ay+h),       // left of anchor (top-right at anchor)
		image.Rect(ax-w/2, ay, ax+w/2, ay+h), // below anchor (centered)
		image.Rect(ax-w/2, ay-h, ax+w/2, ay), // above anchor (centered)
	}

	bestIdx := -1
	bestOverlap := -1
	var bestRect image.Rectangle
	for i, c := range candidates {
		c = clampRectToPane(c, gridW, gridH)
		c = avoidReservedRect(c, gearZone, gridW, gridH)

		overlap := 0
		for _, mr := range members {
			overlap += rectIntersectArea(c, mr)
		}
		if overlap == 0 {
			return c // first clean candidate wins — deterministic priority order
		}
		if bestIdx == -1 || overlap < bestOverlap {
			bestIdx, bestOverlap, bestRect = i, overlap, c
		}
	}
	return bestRect
}

func rectIntersectArea(a, b image.Rectangle) int {
	r := a.Intersect(b)
	if r.Empty() {
		return 0
	}
	return r.Dx() * r.Dy()
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

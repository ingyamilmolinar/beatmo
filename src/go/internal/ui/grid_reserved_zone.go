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

// placeGroupMenu is the deterministic, label-proximal panel-placement
// chooser for GroupMenu (task-placement-v2; replaces the old avoidRect-based
// obstacle-relative chain, whose bugs are documented in
// task-placement-v2-report.md — H1: the old avoidRect's obstacle-relative
// candidates could ALL fail to fit a wide/tall member bbox, falling back to
// the already-overlapping panel even when an anchor-adjacent gap existed;
// H2: the trailing avoidReservedRect re-application could push an
// avoidRect-cleared panel straight back onto a member, since avoidRect had no
// way to know its own answer would be invalidated).
//
// Candidates are positioned relative to the ANCHOR point (not the obstacle),
// in fixed priority order — right of anchor, left of anchor (panel's right
// edge at the anchor), below anchor (centered under it), above anchor
// (centered over it) — because the menu must stay visually adjacent to the
// label/badge that opened it in every outcome, never jump to some
// bbox-relative position that could be far away. Each candidate is pane-
// clamped, then gear-adjusted (avoidReservedRect) — the adjustment can slide
// the candidate down/left but never teleport it, so adjacency is preserved.
//
// The first candidate whose FINAL rect (post clamp, post gear-adjust)
// overlaps NO entry in members wins — tested per-member, not against a
// coarse union bbox: an L-shaped group's bbox is over-conservative and can
// mask a real gap between members (this is what fixes H1). Because the
// per-member check runs AFTER the gear adjustment, a gear-push that
// reintroduces a member overlap is rejected and the next candidate is tried
// instead of being returned regardless (this is what fixes H2).
//
// If every candidate still overlaps at least one member after gear-
// adjustment, falls back to the candidate with the smallest total overlap
// area (sum of intersection areas across members) — the only case the
// product spec allows the panel to cover a member, and even then it's one of
// the four anchor-adjacent candidates, never a bbox-relative position.
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

// rectIntersectArea returns the pixel area of a's intersection with b (0 when
// disjoint). Shared by placeGroupMenu's min-overlap fallback scoring.
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

package ui

import "math"

// sidebar_icons.go houses pure layout/math helpers for the small icons
// drawn inside the node parameter sidebar (right-pointing and down-
// pointing filled triangles used as expand/collapse indicators).
//
// The corresponding NodeSidebar.drawFilledTriangle* methods are thin
// wrappers that call these helpers per scanline, then issue draw rects.
// Splitting the math out keeps the geometry testable without an
// *ebiten.Image, and makes a future pixel-accurate refactor easier
// (one source of truth for the shape).

// triangleRightRowSpan returns the [xStart, xEnd) horizontal span of the
// `row`-th scanline of a right-pointing filled triangle anchored at
// (x, y) with size (w, h). The triangle's tip is at the right edge,
// vertically centered. xEnd <= xStart means "this row is empty".
//
// Spec mirrors NodeSidebar.drawFilledTriangleRight: the leftmost column
// is at x for every row; the right edge approaches x+w at the vertical
// midline and tapers back toward x at the top and bottom. Implementation
// uses the same `1 - dist/halfHeight` formulation so the shape matches
// the legacy in-place loop pixel-for-pixel.
func triangleRightRowSpan(x, y, w, h, row int) (xStart, xEnd int) {
	if w <= 0 || h <= 0 || row < 0 || row >= h {
		return x, x
	}
	mid := float64(h-1) / 2
	dist := math.Abs(float64(row) - mid)
	t := 1 - dist/math.Max(mid, 1)
	if t < 0 {
		t = 0
	}
	return x, x + int(math.Round(t*float64(w)))
}

// triangleDownRowSpan returns the [xStart, xEnd) span of the `row`-th
// scanline of a down-pointing filled triangle anchored at (x, y) with
// size (w, h). Top row is full width; rows narrow linearly to a single
// pixel at the bottom-center.
//
// Mirrors NodeSidebar.drawFilledTriangleDown — the only place the legacy
// version differed is the very last row which falls back to a single
// pixel at the tip; this helper returns that span explicitly so the
// drawing code stays branch-free.
func triangleDownRowSpan(x, y, w, h, row int) (xStart, xEnd int) {
	if w <= 0 || h <= 0 || row < 0 || row >= h {
		return x, x
	}
	t := 1 - float64(row)/math.Max(float64(h-1), 1)
	halfW := int(math.Round(t * float64(w) / 2))
	cx := x + w/2
	if halfW <= 0 {
		// Tip pixel at the bottom-center.
		return cx, cx + 1
	}
	return cx - halfW, cx + halfW
}

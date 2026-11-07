package ui

import "math"

// Kept as standalone, pixel-tested geometry; the sidebar now uses the shared chevron.

// xEnd <= xStart means the row is empty.
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

// The last row is the single tip pixel, returned explicitly so drawing stays branch-free.
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

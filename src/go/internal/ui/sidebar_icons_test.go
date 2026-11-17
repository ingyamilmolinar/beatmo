package ui

import "testing"

// sidebar_icons_test.go covers the pure span calculators that back the
// sidebar's expand/collapse triangle drawing. Tests focus on geometric
// invariants — symmetry around the midline, monotonic taper, single-
// pixel tip — rather than per-pixel snapshots.

func TestTriangleRightRowSpanShape(t *testing.T) {
	const x, y, w, h = 10, 0, 8, 5
	// Every row's left edge is x; right edge >= x and <= x+w.
	for row := 0; row < h; row++ {
		xs, xe := triangleRightRowSpan(x, y, w, h, row)
		if xs != x {
			t.Errorf("row %d: xs=%d want %d", row, xs, x)
		}
		if xe < x {
			t.Errorf("row %d: xe=%d < x=%d", row, xe, x)
		}
		if xe > x+w {
			t.Errorf("row %d: xe=%d > x+w=%d", row, xe, x+w)
		}
	}
	// Mid row reaches the rightmost extent.
	_, xeMid := triangleRightRowSpan(x, y, w, h, h/2)
	if xeMid != x+w {
		t.Errorf("mid row xe=%d want %d (full width)", xeMid, x+w)
	}
	// Top and bottom rows degenerate to xs == xe (empty span).
	_, xeTop := triangleRightRowSpan(x, y, w, h, 0)
	_, xeBot := triangleRightRowSpan(x, y, w, h, h-1)
	if xeTop != x {
		t.Errorf("top row should be empty: xe=%d want %d", xeTop, x)
	}
	if xeBot != x {
		t.Errorf("bottom row should be empty: xe=%d want %d", xeBot, x)
	}
	// Symmetric about the midline (when h is odd).
	for offset := 1; offset <= h/2; offset++ {
		_, above := triangleRightRowSpan(x, y, w, h, h/2-offset)
		_, below := triangleRightRowSpan(x, y, w, h, h/2+offset)
		if above != below {
			t.Errorf("asymmetric: row %d → xe=%d, row %d → xe=%d",
				h/2-offset, above, h/2+offset, below)
		}
	}
}

func TestTriangleRightRowSpanInvalid(t *testing.T) {
	xs, xe := triangleRightRowSpan(5, 0, 0, 5, 2)
	if xs != 5 || xe != 5 {
		t.Errorf("zero width: %d..%d want 5..5", xs, xe)
	}
	xs, xe = triangleRightRowSpan(5, 0, 4, 0, 2)
	if xs != 5 || xe != 5 {
		t.Errorf("zero height: %d..%d want 5..5", xs, xe)
	}
	xs, xe = triangleRightRowSpan(5, 0, 4, 4, -1)
	if xs != 5 || xe != 5 {
		t.Errorf("negative row: %d..%d want 5..5", xs, xe)
	}
	xs, xe = triangleRightRowSpan(5, 0, 4, 4, 4)
	if xs != 5 || xe != 5 {
		t.Errorf("row >= h: %d..%d want 5..5", xs, xe)
	}
}

func TestTriangleDownRowSpanShape(t *testing.T) {
	const x, y, w, h = 0, 0, 10, 6
	cx := x + w/2

	// Top row spans roughly the full width centered on cx.
	xs0, xe0 := triangleDownRowSpan(x, y, w, h, 0)
	if (xe0 - xs0) < w-2 {
		t.Errorf("top row too narrow: %d..%d (width %d, need >= %d)",
			xs0, xe0, xe0-xs0, w-2)
	}
	if cx != (xs0+xe0)/2 {
		// Allow ±1 due to integer rounding.
		if got := (xs0 + xe0) / 2; got != cx && got != cx-1 && got != cx+1 {
			t.Errorf("top row not centered on cx=%d: mid=%d", cx, got)
		}
	}

	// Bottom row is the single-pixel tip at cx.
	xsBot, xeBot := triangleDownRowSpan(x, y, w, h, h-1)
	if xeBot-xsBot != 1 {
		t.Errorf("bottom row should be 1px wide: %d..%d", xsBot, xeBot)
	}
	if xsBot != cx {
		t.Errorf("bottom tip x=%d want %d", xsBot, cx)
	}

	// Width must monotonically narrow from top to bottom.
	prevWidth := -1
	for row := 0; row < h; row++ {
		xs, xe := triangleDownRowSpan(x, y, w, h, row)
		width := xe - xs
		if prevWidth >= 0 && width > prevWidth {
			t.Errorf("row %d wider (%d) than prev (%d)", row, width, prevWidth)
		}
		prevWidth = width
	}
}

func TestTriangleDownRowSpanInvalid(t *testing.T) {
	xs, xe := triangleDownRowSpan(3, 0, 0, 5, 2)
	if xs != 3 || xe != 3 {
		t.Errorf("zero w: %d..%d", xs, xe)
	}
	xs, xe = triangleDownRowSpan(3, 0, 4, 0, 2)
	if xs != 3 || xe != 3 {
		t.Errorf("zero h: %d..%d", xs, xe)
	}
	xs, xe = triangleDownRowSpan(3, 0, 4, 4, 4)
	if xs != 3 || xe != 3 {
		t.Errorf("row out of range: %d..%d", xs, xe)
	}
}

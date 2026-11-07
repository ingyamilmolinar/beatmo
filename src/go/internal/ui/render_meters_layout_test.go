//go:build test

package ui

import (
	"image"
	"testing"
)

func aggRowSpan(r levelsAggRow) (int, int) {
	return r.Y, r.Y + StyledTextHeight(r.Role)
}

// Rows flow strictly downward, never overlap and never pass rect.Max.Y; fixed offsets once overlapped.
func TestLevelsAggregateColumnRows_NoOverlap(t *testing.T) {
	rect := image.Rect(0, 0, 160, 400)
	rows := levelsAggregateColumnRows(rect)
	if len(rows) != aggRowCount {
		t.Fatalf("tall rect must fit all %d rows, got %d", aggRowCount, len(rows))
	}
	for i := 1; i < len(rows); i++ {
		_, prevBot := aggRowSpan(rows[i-1])
		curTop, _ := aggRowSpan(rows[i])
		if rows[i].Y <= rows[i-1].Y {
			t.Fatalf("row %d y=%d not strictly below row %d y=%d", i, rows[i].Y, i-1, rows[i-1].Y)
		}
		if curTop < prevBot {
			t.Fatalf("row %d span starts at %d, overlapping row %d which ends at %d", i, curTop, i-1, prevBot)
		}
	}
	for i, r := range rows {
		_, bot := aggRowSpan(r)
		if bot > rect.Max.Y {
			t.Fatalf("row %d extends to %d past rect.Max.Y=%d", i, bot, rect.Max.Y)
		}
	}
}

// Too short for all rows, it drops trailing rows rather than overlapping; every returned row fits.
func TestLevelsAggregateColumnRows_TruncatesNotOverlaps(t *testing.T) {
	for _, h := range []int{0, 10, 30, 50, 70, 90, 110} {
		rect := image.Rect(0, 100, 160, 100+h)
		rows := levelsAggregateColumnRows(rect)
		if len(rows) > aggRowCount {
			t.Fatalf("h=%d: more rows than defined: %d", h, len(rows))
		}
		for i, r := range rows {
			_, bot := aggRowSpan(r)
			if bot > rect.Max.Y {
				t.Fatalf("h=%d: row %d extends to %d past rect.Max.Y=%d", h, i, bot, rect.Max.Y)
			}
			if i > 0 {
				_, prevBot := aggRowSpan(rows[i-1])
				if r.Y < prevBot {
					t.Fatalf("h=%d: row %d overlaps row %d", h, i, i-1)
				}
			}
		}
	}
	// The label renders first.
	twoRowH := StyledTextHeight(RoleCaption) + levelsAggRowGap + StyledTextHeight(RolePanelTitle)
	rect := image.Rect(0, 0, 160, twoRowH)
	rows := levelsAggregateColumnRows(rect)
	if len(rows) != 2 {
		t.Fatalf("rect of height %d must fit exactly the 2 headroom rows, got %d", twoRowH, len(rows))
	}
}

// Row heights are role-driven, but Spacious sizes the readout column, so pin the pair.
func TestLevelsAggregateColumnRows_SpaciousDensity(t *testing.T) {
	restore := SetDensityForTest(DensitySpacious)
	defer restore()

	w := Profile().DensityValues().LevelsReadoutWFull
	if w <= 0 {
		t.Fatalf("Spacious LevelsReadoutWFull must be positive, got %d", w)
	}
	rect := image.Rect(0, 0, w, 240)
	rows := levelsAggregateColumnRows(rect)
	if len(rows) == 0 {
		t.Fatalf("expected at least the HEADROOM rows")
	}
	for i := 1; i < len(rows); i++ {
		_, prevBot := aggRowSpan(rows[i-1])
		if rows[i].Y < prevBot {
			t.Fatalf("row %d (y=%d) overlaps row %d (ends %d)", i, rows[i].Y, i-1, prevBot)
		}
	}
}

// Truncated to its strip, so a long name cannot bleed into the neighbour.
func TestLevelsChannelStripLabelTruncated(t *testing.T) {
	const stripW = 24
	long := "fm-epiano-1"
	capW := func(s string) int { return StyledTextWidth(s, RoleCaption) }
	got := clipTextToWidthMeasure(long, stripW-2, capW)
	if w := capW(got); w > stripW-2 {
		t.Fatalf("truncated label %q still %dpx wide, exceeds strip budget %d", got, w, stripW-2)
	}
	if got == long && capW(long) > stripW-2 {
		t.Fatalf("long label was not truncated")
	}
}

//go:build test

package ui

import (
	"image"
	"math"
	"testing"
)

// aggRowSpan returns the [top, bottom) pixel range a row occupies given
// the text height used by the layout.
func aggRowSpan(r levelsAggRow, textH int) (int, int) {
	return r.Y, r.Y + int(math.Ceil(float64(textH)*r.Scale))
}

// TestLevelsAggregateColumnRows_NoOverlap pins the aggregate-column
// layout contract: rows flow sequentially (strictly increasing y),
// each row's pixel span never overlaps the next, and no rendered row
// extends past rect.Max.Y. The pre-fix layout used hardcoded absolute
// offsets from the CLIPS row (LUFS value at +40, LOUDEST label at +36)
// which physically overlapped.
func TestLevelsAggregateColumnRows_NoOverlap(t *testing.T) {
	rect := image.Rect(0, 0, 160, 400)
	// Representative TextHeight values: the test-stub debugCharH-style
	// small metric, the desktop body-font metric, and a large metric
	// (Spacious density / hi-dpi font) that broke the old fixed
	// offsets hardest.
	for _, textH := range []int{8, 12, 14, 16, 20, 24} {
		rows := levelsAggregateColumnRows(rect, textH)
		if len(rows) != aggRowCount {
			t.Fatalf("textH=%d: tall rect must fit all %d rows, got %d", textH, aggRowCount, len(rows))
		}
		for i := 1; i < len(rows); i++ {
			_, prevBot := aggRowSpan(rows[i-1], textH)
			curTop, _ := aggRowSpan(rows[i], textH)
			if rows[i].Y <= rows[i-1].Y {
				t.Fatalf("textH=%d: row %d y=%d not strictly below row %d y=%d", textH, i, rows[i].Y, i-1, rows[i-1].Y)
			}
			if curTop < prevBot {
				t.Fatalf("textH=%d: row %d span starts at %d, overlapping row %d which ends at %d", textH, i, curTop, i-1, prevBot)
			}
		}
		for i, r := range rows {
			_, bot := aggRowSpan(r, textH)
			if bot > rect.Max.Y {
				t.Fatalf("textH=%d: row %d extends to %d past rect.Max.Y=%d", textH, i, bot, rect.Max.Y)
			}
		}
	}
}

// TestLevelsAggregateColumnRows_TruncatesNotOverlaps: when the rect is
// too short for all eight rows, the layout drops trailing rows instead
// of overlapping or overflowing. Every returned row must still fit.
func TestLevelsAggregateColumnRows_TruncatesNotOverlaps(t *testing.T) {
	const textH = 14
	for _, h := range []int{0, 10, 30, 50, 70, 90, 110} {
		rect := image.Rect(0, 100, 160, 100+h)
		rows := levelsAggregateColumnRows(rect, textH)
		if len(rows) > aggRowCount {
			t.Fatalf("h=%d: more rows than defined: %d", h, len(rows))
		}
		for i, r := range rows {
			_, bot := aggRowSpan(r, textH)
			if bot > rect.Max.Y {
				t.Fatalf("h=%d: row %d extends to %d past rect.Max.Y=%d", h, i, bot, rect.Max.Y)
			}
			if i > 0 {
				_, prevBot := aggRowSpan(rows[i-1], textH)
				if r.Y < prevBot {
					t.Fatalf("h=%d: row %d overlaps row %d", h, i, i-1)
				}
			}
		}
	}
	// A rect tall enough for only the HEADROOM label + big number must
	// keep exactly those two rows (label is the first to render).
	capScale := FontSizeCaption / FontSizeBody
	twoRowH := int(math.Ceil(float64(textH)*capScale)) + levelsAggRowGap + int(math.Ceil(float64(textH)*2.0))
	rect := image.Rect(0, 0, 160, twoRowH)
	rows := levelsAggregateColumnRows(rect, textH)
	if len(rows) != 2 {
		t.Fatalf("rect of height %d must fit exactly the 2 headroom rows, got %d", twoRowH, len(rows))
	}
}

// TestLevelsAggregateColumnRows_SpaciousDensity exercises the layout
// under the Spacious density profile (mobile default) to make sure the
// density override does not reintroduce overlap. The layout depends on
// TextHeight (not density directly), but the density-sized readout
// column width is where Spacious screens render this column, so pin
// the combination.
func TestLevelsAggregateColumnRows_SpaciousDensity(t *testing.T) {
	restore := SetDensityForTest(DensitySpacious)
	defer restore()

	w := Profile().DensityValues().LevelsReadoutWFull
	if w <= 0 {
		t.Fatalf("Spacious LevelsReadoutWFull must be positive, got %d", w)
	}
	rect := image.Rect(0, 0, w, 240)
	for _, textH := range []int{TextHeight(), 18, 22} {
		rows := levelsAggregateColumnRows(rect, textH)
		if len(rows) == 0 {
			t.Fatalf("textH=%d: expected at least the HEADROOM rows", textH)
		}
		for i := 1; i < len(rows); i++ {
			_, prevBot := aggRowSpan(rows[i-1], textH)
			if rows[i].Y < prevBot {
				t.Fatalf("textH=%d: row %d (y=%d) overlaps row %d (ends %d)", textH, i, rows[i].Y, i-1, prevBot)
			}
		}
	}
}

// TestLevelsChannelStripLabelTruncated: a long channel name must be
// truncated to its strip width so it cannot bleed into the neighbor
// strip (the "CowbellFm-epiano-1" collision).
func TestLevelsChannelStripLabelTruncated(t *testing.T) {
	captionScale := FontSizeCaption / FontSizeBody
	const stripW = 24
	long := "fm-epiano-1"
	got := truncateName(long, stripW-2, captionScale)
	if w := int(float64(TextWidth(got)) * captionScale); w > stripW-2 {
		t.Fatalf("truncated label %q still %dpx wide, exceeds strip budget %d", got, w, stripW-2)
	}
	if got == long && int(float64(TextWidth(long))*captionScale) > stripW-2 {
		t.Fatalf("long label was not truncated")
	}
}

//go:build test

package ui

import (
	"image"
	"testing"
)

// The aggregate column's "headline" row (the big Headroom number) must use a
// scale derived from the type scale, not a raw 2.0 multiplier. After the fix
// it should equal FontSizeHeading/FontSizeBody and be smaller than the old 2.0.
func TestLevelsAggregate_HeadlineUsesTypeScale(t *testing.T) {
	rect := image.Rect(0, 0, 200, 400)
	rows := levelsAggregateColumnRows(rect, 14)
	want := FontSizeHeading / FontSizeBody
	got := rows[aggRowHeadroomValue].Scale
	if got != want {
		t.Fatalf("headline scale: got %v, want FontSizeHeading/FontSizeBody=%v", got, want)
	}
	if got >= 2.0 {
		t.Fatalf("headline scale should be < 2.0 after compaction; got %v", got)
	}
}

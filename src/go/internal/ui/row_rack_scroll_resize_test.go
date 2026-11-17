//go:build test

package ui

import (
	"image"
	"testing"
)

// TestRowRackResizeClampsStaleOffset reproduces the mobile bug where scrolling
// down to reveal the last instrument row (hiding the first row) and then
// resizing the bottom panel so every row fits again leaves the row scroll
// offset stale: the scrollbar correctly disappears (all rows fit) but the
// first row stays hidden because rowOffset is never re-clamped when the rack
// gains vertical space.
func TestRowRackResizeClampsStaleOffset(t *testing.T) {
	restore := noInputForTest()
	defer restore()

	rows := makeTestRows(8)
	z, _ := newTestRowRackZone(rows)

	rh := z.rowHeight()
	if rh <= 0 {
		t.Fatalf("invalid row height %d", rh)
	}

	// Small rack: only a few rows fit, so the kit must scroll.
	smallRect := image.Rect(0, 0, 300, 4*rh)
	tree := registerRowRackZone(z, smallRect)
	tree.Update()

	if z.VisibleRows() >= len(rows) {
		t.Fatalf("test precondition: expected fewer visible rows than total; visible=%d total=%d",
			z.VisibleRows(), len(rows))
	}

	// Scroll all the way down so the first row is hidden.
	scrollArea := findHitAreaByTagPrefix(z.HitAreas(), "row-rack-scroll")
	if scrollArea == nil {
		t.Fatal("expected row-rack-scroll hit area while content overflows")
	}
	for i := 0; i < len(rows)+2; i++ {
		scrollArea.Handler.OnWheel(150, 50, -1)
	}
	tree.Update()

	if z.RowOffset() == 0 {
		t.Fatalf("test precondition: expected non-zero row offset after scrolling down, got 0")
	}

	// Resize the panel taller so every row now fits — the scrollbar disappears.
	bigRect := image.Rect(0, 0, 300, (len(rows)+2)*rh)
	tree.SetZoneRect("row-rack", bigRect)
	tree.Update()

	if z.VisibleRows() < len(rows) {
		t.Fatalf("test precondition: expected all rows to fit after resize; visible=%d total=%d",
			z.VisibleRows(), len(rows))
	}

	// The offset must be clamped back to 0 so the first row is visible again.
	if z.RowOffset() != 0 {
		t.Errorf("row offset not clamped after resize: got %d, want 0 (first row stays hidden)", z.RowOffset())
	}
}

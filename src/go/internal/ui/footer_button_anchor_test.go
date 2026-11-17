//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/log"
)

// footerCaptures takes a screenshot of all footer-strip button rects so the
// anchor tests below can compare layouts before and after a row-count change.
type footerCapture struct {
	addRow  image.Rectangle
	zoomDec image.Rectangle
	zoomInc image.Rectangle
}

func captureFooter(t *testing.T, g *Game) footerCapture {
	t.Helper()
	dv := g.drum
	if dv == nil {
		t.Fatal("DrumView nil")
	}
	c := footerCapture{}
	if dv.rowRackZone != nil {
		c.addRow = dv.rowRackZone.AddRowBtnRect()
	}
	if dv.rowZoomDecBtn != nil {
		c.zoomDec = dv.rowZoomDecBtn.Rect()
	}
	if dv.rowZoomIncBtn != nil {
		c.zoomInc = dv.rowZoomIncBtn.Rect()
	}
	return c
}

// addRowFollowsRows asserts the add-row ghost row's contract for the
// current layout: while the visible rows leave room it sits exactly one slot
// UNDER the last visible row and spans the same content extent as the rows
// (label left edge → kebab right edge), so it reads as "one more row". It
// used to pin to the rack bottom regardless of row count, which opened an
// empty strip between the last row and a floating "+" whenever the drum pane
// was taller than the rows.
func addRowFollowsRows(t *testing.T, g *Game, label string) image.Rectangle {
	t.Helper()
	dv := g.drum
	z := dv.rowRackZone
	add := z.AddRowBtnRect()
	if add.Empty() {
		t.Fatalf("%s: addRow rect empty", label)
	}
	labels, menus := z.RowLabels(), z.RowMenuBtns()
	last := -1
	for i, l := range labels {
		if !l.Rect().Empty() {
			last = i
		}
	}
	if last < 0 {
		t.Fatalf("%s: no visible row", label)
	}
	lr, mr := labels[last].Rect(), menus[last].Rect()
	rh := dv.rowHeight()
	if add.Min.Y != lr.Min.Y+rh {
		t.Errorf("%s: addRow %v is not one slot under the last visible row %v (want Min.Y %d)",
			label, add, lr, lr.Min.Y+rh)
	}
	if add.Dy() != lr.Dy() {
		t.Errorf("%s: addRow height %d != row control height %d", label, add.Dy(), lr.Dy())
	}
	if add.Min.X != lr.Min.X || add.Max.X != mr.Max.X {
		t.Errorf("%s: addRow span [%d,%d) != row span [%d,%d)", label, add.Min.X, add.Max.X, lr.Min.X, mr.Max.X)
	}
	return add
}

func addRows(g *Game, n int) {
	for i := 0; i < n; i++ {
		g.drum.AddRow()
	}
	g.drum.refreshWidgetLayout()
	g.drum.recalcButtons()
	g.drum.calcLayout()
}

// TestAddRowButtonFollowsRowsThenPins (mobile + desktop): the ghost row
// tracks the last visible row while there is room; once the rows overflow and
// scroll it stays under the last VISIBLE row and stops moving as more rows
// are added (a user clicking "+" repeatedly past saturation keeps the same
// target).
func TestAddRowButtonFollowsRowsThenPins(t *testing.T) {
	for _, mobile := range []bool{false, true} {
		name := "desktop"
		if mobile {
			name = "mobile"
		}
		t.Run(name, func(t *testing.T) {
			if mobile {
				setupMobileTest(t, true)
			} else {
				assertDefaultParityState(t)
				withDefaultStart(t, false)
			}
			logger := log.New(testLogOutput(), log.LevelInfo)
			g := New(logger)
			t.Cleanup(g.CloseForTest)
			if mobile {
				g.Layout(390, 844)
			} else {
				g.Layout(1280, 720)
			}
			addRowFollowsRows(t, g, "initial rows")

			// Saturate: far more rows than fit, then more still.
			addRows(g, 25)
			z := g.drum.rowRackZone
			if len(g.drum.Rows) <= z.VisibleRows() {
				t.Fatalf("precondition: %d rows should overflow %d visible", len(g.drum.Rows), z.VisibleRows())
			}
			sat := z.AddRowBtnRect()
			addRows(g, 10)
			if more := z.AddRowBtnRect(); more != sat {
				t.Errorf("addRow moved after saturation: %v -> %v", sat, more)
			}
			// Still flush under the last VISIBLE row (the modulo slack of a
			// rack that is not a whole number of rows tall falls BELOW the
			// ghost row, never between it and the rows), and inside the rack.
			addRowFollowsRows(t, g, "saturated")
			if sat.Max.Y > z.rect.Max.Y {
				t.Errorf("saturated addRow %v extends past the rack %v", sat, z.rect)
			}
		})
	}
}

// TestFooterButtonsAboveBottomActionBar asserts the addRow + zoom chips
// sit ABOVE the mobile bottom action bar (Pads/EQ/Wave/Spec/Mtr/Scope)
// — never overlapping its surface. The first anchor pass put the
// rack rect's Max.Y at dv.Bounds.Max.Y, which made addRowBtn land
// exactly on top of the segmented switcher (regression caught in
// screenshot review 2026-05-10). The fix clamps the rack rect's
// Max.Y to dv.rowsBottom() so the WidgetBoard's rack column ends
// above the bottom action bar.
func TestFooterButtonsAboveBottomActionBar(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)

	dv := g.drum
	bar := dv.bottomActionBarRect
	if bar.Empty() {
		t.Fatal("preconditions: bottomActionBarRect should be non-empty on this layout")
	}
	checks := []struct {
		name string
		r    image.Rectangle
	}{
		{"addRow", dv.rowRackZone.AddRowBtnRect()},
		{"zoomDec", dv.rowZoomDecBtn.Rect()},
		{"zoomInc", dv.rowZoomIncBtn.Rect()},
	}
	for _, c := range checks {
		if c.r.Empty() {
			continue
		}
		if c.r.Overlaps(bar) {
			t.Errorf("%s rect %v overlaps bottom action bar %v — footer button is rendered on top of the segmented switcher",
				c.name, c.r, bar)
		}
		if c.r.Max.Y > bar.Min.Y {
			t.Errorf("%s rect %v extends below bar top (Y=%d) — must stay strictly above the bar",
				c.name, c.r, bar.Min.Y)
		}
	}

	// Sanity: the rack ZONE rect (after the rowsBottom() clamp in
	// drumview_layout.go) must end at or above bar.Min.Y. The raw
	// WidgetBoard rect (dv.widgetRects[WidgetRack]) extends to
	// dv.Bounds.Max.Y by design — only the clamped zone rect knows
	// about the bottom action bar.
	zoneRect := dv.rowRackZone.rect
	if zoneRect.Max.Y > bar.Min.Y {
		t.Errorf("rowRackZone.rect.Max.Y=%d extends past bar top Y=%d — rack zone rect is overlapping the bottom action bar",
			zoneRect.Max.Y, bar.Min.Y)
	}
}

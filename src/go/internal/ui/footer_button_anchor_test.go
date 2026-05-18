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

// TestAddRowButtonAnchoredAcrossRowCounts asserts the addRow button
// stays put once the rack is saturated. The 5-row and 20-row layouts
// must produce identical addRow / zoom-chip rects so users who keep
// clicking "+" after the rack has filled don't see the target move.
//
// This is the load-bearing portion of the original "no moving target"
// invariant. The 1-row pre-saturation case used to be tied here too;
// the 2026-05-10 tight-layout fix relaxes it: when the rack has more
// than one row of slack, addRow pins to the rack bottom; when slack
// drops below one row (near-saturation), it snaps flush below the
// last row to eliminate the orphan modulo gap flagged in the
// screenshot review. See `mobile_default_unified_layout_test.go`'s
// `TestMobileDefault_AddRowBtnFlushBelowLastRow` for the snap
// invariant. The transition is a single discrete jump as the rack
// crosses the modulo threshold.
func TestAddRowButtonAnchoredAcrossRowCounts(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)

	one := captureFooter(t, g)
	if one.addRow.Empty() {
		t.Fatal("addRow rect empty after Layout — preconditions failed")
	}

	// Add 4 more rows → 5 total. The rack saturates around here on a
	// 390x844 viewport; addRow snaps from bottom-anchor to flush.
	for i := 0; i < 4; i++ {
		g.drum.AddRow()
	}
	g.drum.refreshWidgetLayout()
	g.drum.recalcButtons()
	g.drum.calcLayout()
	five := captureFooter(t, g)

	// Add 15 more rows → 20 total (forces scroll).
	for i := 0; i < 15; i++ {
		g.drum.AddRow()
	}
	g.drum.refreshWidgetLayout()
	g.drum.recalcButtons()
	g.drum.calcLayout()
	twenty := captureFooter(t, g)

	// Saturated stability: 5-row == 20-row. Adding rows past saturation
	// must not move the target.
	if five.addRow != twenty.addRow {
		t.Errorf("addRow moved after saturation (5 vs 20 rows): 5-row=%v 20-row=%v",
			five.addRow, twenty.addRow)
	}
	if five.zoomDec != twenty.zoomDec {
		t.Errorf("zoomDec moved after saturation (5 vs 20 rows): 5=%v 20=%v",
			five.zoomDec, twenty.zoomDec)
	}
	if five.zoomInc != twenty.zoomInc {
		t.Errorf("zoomInc moved after saturation (5 vs 20 rows): 5=%v 20=%v",
			five.zoomInc, twenty.zoomInc)
	}

	// 1-row → 5-row transition: addRow may move at most by one rh as it
	// snaps from bottom-anchor to flush. Anything larger signals a bug
	// (e.g., button drifting downward each click — the original concern).
	rh := g.drum.rowHeight()
	jump := absInt(one.addRow.Min.Y - five.addRow.Min.Y)
	if jump > rh {
		t.Errorf("addRow moved more than one rh between 1 and 5 rows: 1-row=%v 5-row=%v jump=%dpx (max=%d)",
			one.addRow, five.addRow, jump, rh)
	}
}


// TestAddRowButtonAnchoredAcrossRowCountsDesktop covers the strict
// "no movement across row counts" invariant on desktop. Desktop keeps
// the original bottom-anchor everywhere (the snap-flush behavior is
// gated to mobile in `positionAddRowBtn`), so 1, 5, and 20 row layouts
// must produce identical addRow rects regardless of saturation.
func TestAddRowButtonAnchoredAcrossRowCountsDesktop(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)

	one := captureFooter(t, g)
	if one.addRow.Empty() {
		t.Fatal("desktop addRow rect empty after Layout")
	}

	for i := 0; i < 4; i++ {
		g.drum.AddRow()
	}
	g.drum.refreshWidgetLayout()
	g.drum.recalcButtons()
	g.drum.calcLayout()
	five := captureFooter(t, g)

	for i := 0; i < 15; i++ {
		g.drum.AddRow()
	}
	g.drum.refreshWidgetLayout()
	g.drum.recalcButtons()
	g.drum.calcLayout()
	twenty := captureFooter(t, g)

	if one.addRow != five.addRow || one.addRow != twenty.addRow {
		t.Errorf("desktop addRow moved across row counts: 1=%v 5=%v 20=%v",
			one.addRow, five.addRow, twenty.addRow)
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

// TestAddRowButtonPinnedToBottom asserts the addRowBtn sits flush against
// the rack column's USABLE bottom edge — `dv.rowsBottom()`, which excludes
// the mobile bottom action bar and EQ peek strip. The only stable
// reference point that doesn't move with row count, scroll, or visible-
// row math AND doesn't render on top of the segmented switcher.
func TestAddRowButtonPinnedToBottom(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)

	dv := g.drum
	addRect := dv.rowRackZone.AddRowBtnRect()
	if addRect.Empty() {
		t.Fatal("addRow rect empty")
	}
	rh := dv.rowHeight()
	// rowsBottom() is the authoritative anchor — it subtracts the
	// bottom action bar and EQ peek strip from dv.Bounds.Max.Y. The
	// addRow's top edge (after SpaceXS inset) should be at
	// (rowsBottom() - rh + SpaceXS).
	expectedTop := dv.rowsBottom() - rh
	const slack = 4 // pixels of SpaceXS inset tolerance
	if addRect.Min.Y < expectedTop-slack || addRect.Min.Y > expectedTop+slack+SpaceXS*2 {
		t.Errorf("addRow not pinned to rowsBottom: addRow.Min.Y=%d, expected ≈%d (rowsBottom=%d, rh=%d)",
			addRect.Min.Y, expectedTop, dv.rowsBottom(), rh)
	}
}

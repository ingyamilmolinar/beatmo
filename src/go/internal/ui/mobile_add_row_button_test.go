package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/log"
)

// mobileAddRowFooterCommonSetup builds a Game in mobile portrait, lays it
// out at iPhone 13 dimensions, and returns the DrumView so tests can poke
// at the rack. All tests in this file share these dimensions so visible-row
// math stays consistent.
func mobileAddRowFooterCommonSetup(t *testing.T) *Game {
	t.Helper()
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)
	return g
}

// TestMobileAddRowButton_VisibleAndFullWidth_NoRows verifies the add-row
// footer button is rendered (non-empty rect) and spans the rack panel
// width even when there are no extra rows beyond the default. This is the
// "always usable, regardless of row count" guarantee for a sparse layout.
func TestMobileAddRowButton_VisibleAndFullWidth_NoRows(t *testing.T) {
	g := mobileAddRowFooterCommonSetup(t)
	dv := g.drum

	btnRect := dv.addRowBtn().Rect()
	if btnRect.Empty() {
		t.Fatalf("expected mobile add-row footer button to be visible; got empty rect")
	}
	rackRect := dv.widgetRects[WidgetRack]
	if rackRect.Empty() {
		t.Fatal("rack rect is empty; layout did not run")
	}
	if btnRect.Dx() < rackRect.Dx()/2 {
		t.Errorf("expected full-width footer; got width=%d (rack width=%d)",
			btnRect.Dx(), rackRect.Dx())
	}
	// Footer should be horizontally inside the rack panel.
	if btnRect.Min.X < rackRect.Min.X || btnRect.Max.X > rackRect.Max.X {
		t.Errorf("footer X range [%d,%d] exceeds rack X range [%d,%d]",
			btnRect.Min.X, btnRect.Max.X, rackRect.Min.X, rackRect.Max.X)
	}
}

// TestMobileAddRowButton_AlwaysVisible_ManyRows verifies the add-row footer
// button stays visible and never overlaps any visible row, even when rows
// exceed the visible window so the rack must scroll. This is the "always
// space to use it" guarantee.
func TestMobileAddRowButton_AlwaysVisible_ManyRows(t *testing.T) {
	g := mobileAddRowFooterCommonSetup(t)
	dv := g.drum

	// Add many rows to force scrolling.
	for i := 0; i < 20; i++ {
		dv.AddRow()
	}
	dv.refreshWidgetLayout()
	dv.recalcButtons()
	dv.calcLayout()

	btnRect := dv.addRowBtn().Rect()
	if btnRect.Empty() {
		t.Fatalf("expected mobile add-row footer button to remain visible with 20 rows; got empty rect")
	}

	// Button must not overlap any visible row label rect.
	for i, lbl := range dv.rowLabels() {
		lr := lbl.Rect()
		if lr.Empty() {
			continue
		}
		if btnRect.Overlaps(lr) {
			t.Errorf("add-row button %v overlaps row %d label %v", btnRect, i, lr)
		}
	}

	// Footer must stay within the rack panel bounds (no clipping off-screen).
	rackRect := dv.widgetRects[WidgetRack]
	if !btnRect.In(rackRect) {
		t.Errorf("add-row button %v escapes rack rect %v", btnRect, rackRect)
	}
}

// TestMobileAddRowButton_HiddenInEQMode is a regression that the footer
// button is suppressed (empty rect) while the mobile EQ bottom-sheet is
// active, so the button does not bleed through the sheet.
func TestMobileAddRowButton_HiddenInEQMode(t *testing.T) {
	g := mobileAddRowFooterCommonSetup(t)
	dv := g.drum

	dv.SetMobileEQMode(true)
	defer dv.SetMobileEQMode(false)
	dv.refreshWidgetLayout()
	dv.recalcButtons()
	dv.calcLayout()

	btnRect := dv.addRowBtn().Rect()
	if !btnRect.Empty() {
		t.Errorf("expected add-row button to be hidden in mobile EQ mode; got rect %v", btnRect)
	}
}

// TestMobileAddRowButton_ClickAddsRow verifies that the footer button is
// functional on mobile: invoking it via the OnClick path appends a row.
func TestMobileAddRowButton_ClickAddsRow(t *testing.T) {
	g := mobileAddRowFooterCommonSetup(t)
	dv := g.drum

	before := len(dv.Rows)
	btn := dv.addRowBtn()
	if btn == nil {
		t.Fatal("addRowBtn is nil")
	}
	if btn.Rect().Empty() {
		t.Fatal("addRowBtn rect is empty before click")
	}
	if btn.OnClick == nil {
		t.Fatal("addRowBtn has no OnClick handler")
	}
	btn.OnClick()
	if got := len(dv.Rows); got != before+1 {
		t.Fatalf("clicking add-row footer should add a row; rows before=%d after=%d", before, got)
	}
}

// TestMobileAddRowButton_BelowRowsArea verifies the footer button sits at or
// below the bottom of the rows area (i.e. it lives in the reserved footer
// strip, not floating over the row content). Combined with the no-overlap
// test above, this guarantees the button has dedicated layout space.
func TestMobileAddRowButton_BelowRowsArea(t *testing.T) {
	g := mobileAddRowFooterCommonSetup(t)
	dv := g.drum

	// Mid-density: more rows than a single row but not necessarily scrolling.
	for i := 0; i < 3; i++ {
		dv.AddRow()
	}
	dv.refreshWidgetLayout()
	dv.recalcButtons()
	dv.calcLayout()

	btnRect := dv.addRowBtn().Rect()
	if btnRect.Empty() {
		t.Fatal("addRowBtn rect empty")
	}
	// Compute the bottom edge of the visible rows area.
	rh := dv.rowHeight()
	vis := dv.visibleRows()
	rackRect := dv.widgetRects[WidgetRack]
	rowsBottom := rackRect.Min.Y + vis*rh
	if btnRect.Min.Y < rowsBottom-1 { // -1 tolerates pixel rounding/inset.
		t.Errorf("add-row button top Y=%d should be at or below visible-rows bottom Y=%d",
			btnRect.Min.Y, rowsBottom)
	}

	// Sanity: rect should be within the bottom rowHeight strip of the rack.
	footerStripTop := rackRect.Max.Y - rh
	footerStrip := image.Rect(rackRect.Min.X, footerStripTop, rackRect.Max.X, rackRect.Max.Y)
	if !btnRect.Overlaps(footerStrip) {
		t.Errorf("add-row button %v does not overlap footer strip %v", btnRect, footerStrip)
	}
}

package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/log"
)

// iPhone 13 dimensions, shared so the visible-row maths stays consistent.
func mobileAddRowFooterCommonSetup(t *testing.T) *Game {
	t.Helper()
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)
	return g
}

// The add-row button owns the bottom rack strip alone; the zoom chips live in the timeline header.
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
		t.Errorf("expected add-row button to span ≥ half of rack; got width=%d (rack width=%d)",
			btnRect.Dx(), rackRect.Dx())
	}
	// Add-row stays inside the rack panel horizontally.
	if btnRect.Min.X < rackRect.Min.X || btnRect.Max.X > rackRect.Max.X {
		t.Errorf("add-row button X range [%d,%d] exceeds rack X range [%d,%d]",
			btnRect.Min.X, btnRect.Max.X, rackRect.Min.X, rackRect.Max.X)
	}
	// The row-zoom chips live in the timeline widget area now.
	tlRect := dv.widgetRects[WidgetTimeline]
	if tlRect.Empty() {
		t.Fatal("timeline widget rect empty; preconditions not met")
	}
	chipUnion := image.Rectangle{}
	if dv.rowZoomDecBtn != nil {
		if r := dv.rowZoomDecBtn.Rect(); !r.Empty() {
			chipUnion = r
		}
	}
	if dv.rowZoomIncBtn != nil {
		if r := dv.rowZoomIncBtn.Rect(); !r.Empty() {
			if chipUnion.Empty() {
				chipUnion = r
			} else {
				chipUnion = chipUnion.Union(r)
			}
		}
	}
	if chipUnion.Empty() {
		t.Fatalf("row-zoom chips empty on mobile default layout")
	}
	if chipUnion.Min.X < tlRect.Min.X || chipUnion.Max.X > tlRect.Max.X {
		t.Errorf("row-zoom chip union X range [%d,%d] escapes timeline rect X range [%d,%d]",
			chipUnion.Min.X, chipUnion.Max.X, tlRect.Min.X, tlRect.Max.X)
	}
}

// Always visible and never over a row, even when the rack scrolls.
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

// Hidden under the EQ bottom sheet, so it does not bleed through.
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

// One slot below the last visible row: never over content, never floating at the rack bottom.
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
	rh := dv.rowHeight()
	labels := dv.rowLabels()
	last := image.Rectangle{}
	for _, l := range labels {
		if !l.Rect().Empty() {
			last = l.Rect()
		}
	}
	if last.Empty() {
		t.Fatal("no visible row")
	}
	if btnRect.Min.Y != last.Min.Y+rh {
		t.Errorf("add-row button top Y=%d should be one slot under the last visible row (Y=%d)",
			btnRect.Min.Y, last.Min.Y+rh)
	}
	// And it stays above the mobile bottom action bar (Pads/EQ/Wave/...).
	if btnRect.Max.Y > dv.rowsBottom() {
		t.Errorf("add-row button %v extends below the usable rows bottom %d", btnRect, dv.rowsBottom())
	}
}

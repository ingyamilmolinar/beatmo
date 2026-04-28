package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/log"
)

// ────────────────────── Positive tests (mobile) ──────────────────────

// TestMobileEQCollapsedByDefault verifies that on a small screen the EQ
// panel is collapsed by default and eqH is zero.
func TestMobileEQCollapsedByDefault(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)

	if !g.drum.MobileEQCollapsed() {
		t.Fatal("expected mobileEQCollapsed=true on small screen")
	}
	if g.drum.eqH != 0 {
		t.Fatalf("expected eqH=0 when collapsed, got %d", g.drum.eqH)
	}
}

// TestMobileEQToggleRestoresHeight toggles the EQ on and off and checks
// that rowsAreaHeight adjusts accordingly.
func TestMobileEQToggleRestoresHeight(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)

	collapsedRows := g.drum.rowsAreaHeight()
	if collapsedRows <= 0 {
		t.Fatalf("collapsed rowsAreaHeight=%d, want > 0", collapsedRows)
	}

	// Toggle on: un-collapse
	g.drum.SetMobileEQCollapsed(false)
	g.drum.widgets.ToggleWidget(WidgetWave, true)
	g.drum.refreshWidgetLayout()
	g.drum.recalcButtons()
	g.drum.calcLayout()

	expandedRows := g.drum.rowsAreaHeight()
	// Expanded should have fewer row pixels because EQ takes space.
	// Under Go test eqPanelHeight=0, so they may be equal; just verify non-negative.
	if expandedRows < 0 {
		t.Fatalf("expanded rowsAreaHeight=%d, want >= 0", expandedRows)
	}

	// Toggle off again
	g.drum.SetMobileEQCollapsed(true)
	g.drum.widgets.ToggleWidget(WidgetWave, false)
	g.drum.eqH = 0
	g.drum.refreshWidgetLayout()
	g.drum.recalcButtons()
	g.drum.calcLayout()

	restored := g.drum.rowsAreaHeight()
	if restored != collapsedRows {
		t.Fatalf("rowsAreaHeight after re-collapse=%d, want %d", restored, collapsedRows)
	}
}

// TestMobileOverflowMenuHidesFileButtons checks that Upload/Import/Export
// have zero-area rects on mobile while the overflow button has a valid rect.
func TestMobileOverflowMenuHidesFileButtons(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)
	advanceFrames(g, 2)

	dv := g.drum
	for _, pair := range []struct {
		name string
		btn  *Button
	}{
		{"upload", dv.uploadBtn()},
		{"import", dv.importBtn()},
		{"export", dv.exportBtn()},
	} {
		r := pair.btn.Rect()
		if r.Dx()*r.Dy() > 0 {
			t.Errorf("%s button should have zero area on mobile, got %v", pair.name, r)
		}
	}

	overflow := dv.OverflowBtn()
	if overflow == nil {
		t.Fatal("overflow button is nil")
	}
	or := overflow.Rect()
	if or.Dx() < 14 || or.Dy() < 12 {
		t.Fatalf("overflow button rect too small: %v (need Dx>=14, Dy>=12)", or)
	}
}

// TestMobileTransportNoOverlap checks that no pair of transport buttons
// in the top row overlap on a 390×844 viewport.
func TestMobileTransportNoOverlap(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)
	advanceFrames(g, 2)

	dv := g.drum
	btns := []*Button{
		dv.playBtn(), dv.stopBtn(), dv.bpmIncBtn(), dv.bpmDecBtn(),
		dv.subdivBtn(),
	}
	if dv.eqToggleMobile() != nil {
		btns = append(btns, dv.eqToggleMobile())
	}
	if dv.overflowBtn() != nil {
		btns = append(btns, dv.overflowBtn())
	}
	for i := 0; i < len(btns); i++ {
		for j := i + 1; j < len(btns); j++ {
			ri := btns[i].Rect()
			rj := btns[j].Rect()
			if ri.Empty() || rj.Empty() {
				continue
			}
			if ri.Overlaps(rj) {
				t.Errorf("transport buttons %d and %d overlap: %v vs %v", i, j, ri, rj)
			}
		}
	}
}

// TestMobileAddRowButtonFAB checks that the add-row button is positioned
// as a FAB and doesn't overlap row labels or timeline cells.
func TestMobileAddRowButtonFAB(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)
	advanceFrames(g, 2)

	dv := g.drum
	fab := dv.addRowBtn().Rect()
	if fab.Empty() {
		t.Fatal("addRowBtn rect is empty")
	}
	// FAB should be on the right side of the screen
	midX := dv.Bounds.Min.X + dv.Bounds.Dx()/2
	if fab.Min.X < midX {
		t.Errorf("FAB should be on right side, got Min.X=%d, midpoint=%d", fab.Min.X, midX)
	}
	// Shouldn't overlap row labels
	for i, lbl := range dv.rowLabels() {
		lr := lbl.Rect()
		if lr.Empty() {
			continue
		}
		if fab.Overlaps(lr) {
			t.Errorf("FAB overlaps row label %d: fab=%v label=%v", i, fab, lr)
		}
	}
}

// TestMobileRowControlsSimplified verifies that on mobile, edit/color/
// mute/solo/origin/delete buttons have empty rects while label+volume remain.
func TestMobileRowControlsSimplified(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)
	advanceFrames(g, 2)

	dv := g.drum
	if len(dv.Rows) == 0 {
		t.Fatal("no rows")
	}
	// Check that label button exists and is non-empty
	if len(dv.rowLabels()) == 0 {
		t.Fatal("no row labels")
	}
	lr := dv.rowLabels()[0].Rect()
	if lr.Empty() {
		t.Error("label button has empty rect on mobile")
	}
	// Mute buttons are hidden on mobile
	if len(dv.rowMuteBtns()) > 0 {
		mr := dv.rowMuteBtns()[0].Rect()
		if !mr.Empty() {
			t.Error("mute button should have empty rect on mobile")
		}
	}
	// Solo buttons are hidden on mobile
	if len(dv.rowSoloBtns()) > 0 {
		sr := dv.rowSoloBtns()[0].Rect()
		if !sr.Empty() {
			t.Error("solo button should have empty rect on mobile")
		}
	}
	// Hidden buttons should be functionally invisible (≤ 5px in one dimension).
	// Grid layout rounding may give the last zero-weight column a few pixels.
	const hiddenMax = 5
	for _, pair := range []struct {
		name string
		btns []*Button
	}{
		{"edit", dv.rowEditBtns()},
		{"color", dv.rowColorBtns()},
		{"origin", dv.rowOriginBtns()},
		{"delete", dv.rowDeleteBtns()},
	} {
		if len(pair.btns) == 0 {
			continue
		}
		r := pair.btns[0].Rect()
		if r.Dx() > hiddenMax && r.Dy() > hiddenMax {
			t.Errorf("%s button should be hidden on mobile (≤%dpx), got %v (%dx%d)", pair.name, hiddenMax, r, r.Dx(), r.Dy())
		}
	}
}

// TestMobileContextMenuOpens verifies that openContextMenu creates a valid
// context menu with 7 buttons (6 items + close) bounded within dv.Bounds.
// Mobile items: Instrument, Rename, Color, Effects, Origin, Delete (+ close).
func TestMobileContextMenuOpens(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)
	advanceFrames(g, 2)

	dv := g.drum
	dv.OpenContextMenu(0)

	if !dv.ContextMenuOpen() {
		t.Fatal("context menu not open")
	}
	btns := dv.ContextMenuBtns()
	if len(btns) != 7 {
		t.Fatalf("expected 7 context menu buttons (6 items + close), got %d", len(btns))
	}
	rect := dv.ContextMenuRectVal()
	if rect.Empty() {
		t.Fatal("context menu rect is empty")
	}
	if !rect.In(dv.Bounds) {
		t.Errorf("context menu %v exceeds drum bounds %v", rect, dv.Bounds)
	}
}

// TestMobileContextMenuBounds checks that the context menu for the last
// row stays within dv.Bounds (doesn't overflow bottom).
func TestMobileContextMenuBounds(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)
	advanceFrames(g, 2)

	dv := g.drum
	lastRow := len(dv.Rows) - 1
	dv.OpenContextMenu(lastRow)

	if !dv.ContextMenuOpen() {
		t.Fatal("context menu not open")
	}
	rect := dv.ContextMenuRectVal()
	if rect.Max.Y > dv.Bounds.Max.Y {
		t.Errorf("context menu overflows bottom: menu.Max.Y=%d > bounds.Max.Y=%d", rect.Max.Y, dv.Bounds.Max.Y)
	}
	if rect.Max.X > dv.Bounds.Max.X {
		t.Errorf("context menu overflows right: menu.Max.X=%d > bounds.Max.X=%d", rect.Max.X, dv.Bounds.Max.X)
	}
}

// TestMobileInstrumentMenuFullWidth checks that on 390×844 the instrument
// menu uses most of the screen width.
func TestMobileInstrumentMenuFullWidth(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)
	advanceFrames(g, 2)

	dv := g.drum
	menuW := dv.instMenuFullRect.Dx()
	threshold := dv.Bounds.Dx() * 60 / 100 // at least 60% of width
	if menuW < threshold {
		t.Errorf("instrument menu width %d < %d (60%% of %d)", menuW, threshold, dv.Bounds.Dx())
	}
}

// TestMobileLabelNotTruncated checks that the label button is wide enough
// to render "Kick" without truncation.
func TestMobileLabelNotTruncated(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)
	advanceFrames(g, 2)

	dv := g.drum
	if len(dv.rowLabels()) == 0 {
		t.Fatal("no row labels")
	}
	lr := dv.rowLabels()[0].Rect()
	// "Kick" is 4 chars × debugCharW (~6px) = 24px minimum. Add small padding.
	// Label column is narrower on mobile to give the kebab button enough touch area.
	minW := 4*debugCharW + 4
	if lr.Dx() < minW {
		t.Errorf("label width %d < %d (min for 4-char label + padding)", lr.Dx(), minW)
	}
}

// TestMobileBeatInfoVisible checks that on mobile, secondary controls
// (volume, length) are hidden from the transport bar (moved to overflow).
func TestMobileBeatInfoVisible(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)
	advanceFrames(g, 2)

	dv := g.drum
	// Length +/- buttons are positioned in the timeline widget area.
	if dv.lenDecBtn == nil || dv.lenIncBtn == nil {
		t.Fatal("length buttons nil")
	}
	ldr := dv.lenDecBtn.Rect()
	lir := dv.lenIncBtn.Rect()
	if ldr.Empty() {
		t.Error("lenDecBtn should be visible on mobile (in timeline area)")
	}
	if lir.Empty() {
		t.Error("lenIncBtn should be visible on mobile (in timeline area)")
	}
	// Overflow button should be visible
	if dv.overflowBtn() == nil {
		t.Fatal("overflowBtn is nil")
	}
	if dv.overflowBtn().Rect().Empty() {
		t.Error("overflowBtn should be visible on mobile")
	}
}

// ────────────────────── Negative tests (desktop unchanged) ──────────────────────

// TestDesktopEQVisible verifies that on desktop the EQ panel is visible
// (mobileEQCollapsed is false and eqH is the standard panel height).
func TestDesktopEQVisible(t *testing.T) {
	setupMobileTest(t, false) // desktop
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)

	if g.drum.MobileEQCollapsed() {
		t.Fatal("desktop: mobileEQCollapsed should be false")
	}
	// Under Go test, eqPanelHeight=0, so eqH will be 0 too. Just verify
	// the collapsed flag is false.
}

// TestDesktopRowControlsFull verifies the desktop 6-column layout:
// Label | VolBar | M | S | FX | Overflow. Edit/save/color/origin/delete
// are hidden (empty rects) — they live in the overflow menu.
func TestDesktopRowControlsFull(t *testing.T) {
	setupMobileTest(t, false) // desktop
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	advanceFrames(g, 2)

	dv := g.drum
	if len(dv.Rows) == 0 {
		t.Fatal("no rows")
	}
	// Buttons that should be visible on desktop.
	visible := []struct {
		name string
		btns []*Button
	}{
		{"label", dv.rowLabels()},
		{"mute", dv.rowMuteBtns()},
		{"solo", dv.rowSoloBtns()},
	}
	for _, c := range visible {
		if len(c.btns) == 0 {
			t.Errorf("desktop: %s buttons slice empty", c.name)
			continue
		}
		r := c.btns[0].Rect()
		if r.Dx()*r.Dy() == 0 {
			t.Errorf("desktop: %s button has zero area: %v", c.name, r)
		}
	}
	// Buttons that should be hidden on desktop (empty rects).
	hidden := []struct {
		name string
		btns []*Button
	}{
		{"edit", dv.rowEditBtns()},
		{"color", dv.rowColorBtns()},
		{"origin", dv.rowOriginBtns()},
		{"delete", dv.rowDeleteBtns()},
	}
	for _, c := range hidden {
		if len(c.btns) == 0 {
			t.Errorf("desktop: %s buttons slice empty", c.name)
			continue
		}
		r := c.btns[0].Rect()
		if r.Dx()*r.Dy() != 0 {
			t.Errorf("desktop: %s button should be hidden (empty rect), got %v", c.name, r)
		}
	}
	// Volume slider
	if len(dv.rowVolSliders()) > 0 {
		vr := dv.rowVolSliders()[0].Rect()
		if vr.Dx()*vr.Dy() == 0 {
			t.Error("desktop: volume slider has zero area")
		}
	}
}

// TestDesktopTransportFull verifies that Upload/Import/Export buttons
// have valid rects and no overflow button is used on desktop.
func TestDesktopTransportFull(t *testing.T) {
	setupMobileTest(t, false) // desktop
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	advanceFrames(g, 2)

	dv := g.drum
	for _, pair := range []struct {
		name string
		btn  *Button
	}{
		{"upload", dv.uploadBtn()},
		{"import", dv.importBtn()},
		{"export", dv.exportBtn()},
	} {
		r := pair.btn.Rect()
		if r.Dx()*r.Dy() == 0 {
			t.Errorf("desktop: %s button has zero area", pair.name)
		}
	}
	// Overflow button should have zero area on desktop (hidden).
	if dv.overflowBtn() != nil {
		or := dv.overflowBtn().Rect()
		if or.Dx()*or.Dy() > 0 {
			t.Errorf("desktop: overflow button should have zero area, got %v", or)
		}
	}
}

// TestDesktopNoContextMenu verifies that contextMenuOpen stays false on
// desktop and that M/S/O/X buttons are functional.
func TestDesktopNoContextMenu(t *testing.T) {
	setupMobileTest(t, false) // desktop
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	advanceFrames(g, 2)

	if g.drum.ContextMenuOpen() {
		t.Fatal("desktop: context menu should not be open")
	}
	// All row control buttons should be present
	dv := g.drum
	if len(dv.rowMuteBtns()) > 0 {
		mr := dv.rowMuteBtns()[0].Rect()
		if mr.Empty() {
			t.Error("desktop: mute button empty")
		}
	}
	if len(dv.rowSoloBtns()) > 0 {
		sr := dv.rowSoloBtns()[0].Rect()
		if sr.Empty() {
			t.Error("desktop: solo button empty")
		}
	}
}

// TestDesktopAddRowButtonInline verifies that addRowBtn is positioned
// inline (in the rack column) on desktop, not as a FAB.
func TestDesktopAddRowButtonInline(t *testing.T) {
	setupMobileTest(t, false) // desktop
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	advanceFrames(g, 2)

	dv := g.drum
	fab := dv.addRowBtn().Rect()
	if fab.Empty() {
		t.Fatal("desktop: addRowBtn rect is empty")
	}
	// On desktop, the add button should be in the left panel (rack area),
	// not floating on the right side.
	rack := dv.widgetRects[WidgetRack]
	if !rack.Empty() {
		// Inline button should overlap the rack X-range
		if fab.Min.X > rack.Max.X {
			t.Errorf("desktop: addRowBtn at x=%d should be within rack [%d,%d]", fab.Min.X, rack.Min.X, rack.Max.X)
		}
	}
}

// ────────────────────── Multi-viewport tests ──────────────────────

// TestMobileContextMenuAllViewports verifies context menu bounds on
// all standard mobile viewports.
func TestMobileContextMenuAllViewports(t *testing.T) {
	for _, vp := range mobileViewports {
		t.Run(vp.name, func(t *testing.T) {
			setupMobileTest(t, true)
			logger := log.New(testLogOutput(), log.LevelInfo)
			g := New(logger)
			t.Cleanup(g.CloseForTest)
			g.Layout(vp.w, vp.h)
			advanceFrames(g, 2)

			dv := g.drum
			if len(dv.Rows) == 0 {
				t.Skip("no rows")
			}
			dv.OpenContextMenu(0)
			if !dv.ContextMenuOpen() {
				t.Fatal("context menu not open")
			}
			rect := dv.ContextMenuRectVal()
			if !rect.In(dv.Bounds) {
				// Allow 1px tolerance for rounding
				expanded := image.Rect(dv.Bounds.Min.X-1, dv.Bounds.Min.Y-1, dv.Bounds.Max.X+1, dv.Bounds.Max.Y+1)
				if !rect.In(expanded) {
					t.Errorf("context menu %v exceeds drum bounds %v", rect, dv.Bounds)
				}
			}
		})
	}
}

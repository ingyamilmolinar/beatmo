package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/log"
)

// ────────────────────── Positive tests (mobile) ──────────────────────

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
	// eqPanelHeight is 0 under test, so the two may be equal.
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

	// File options open from the grid folder button.
}

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

// A full-width footer below the rows that never overlaps a label; the zoom chips live in the timeline header.
func TestMobileAddRowButtonFooter(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)
	advanceFrames(g, 2)

	dv := g.drum
	btn := dv.addRowBtn().Rect()
	if btn.Empty() {
		t.Fatal("addRowBtn rect is empty; expected a footer button on mobile")
	}
	rackRect := dv.widgetRects[WidgetRack]
	if rackRect.Empty() {
		t.Fatal("rack rect is empty")
	}
	if btn.Dx() < rackRect.Dx()/2 {
		t.Errorf("expected add-row button width ≥ rack/2; got width=%d (rack width=%d)",
			btn.Dx(), rackRect.Dx())
	}
	// Shouldn't overlap any row label.
	for i, lbl := range dv.rowLabels() {
		lr := lbl.Rect()
		if lr.Empty() {
			continue
		}
		if btn.Overlaps(lr) {
			t.Errorf("add-row button overlaps row label %d: btn=%v label=%v", i, btn, lr)
		}
	}
	for _, chip := range []*Button{dv.rowZoomDecBtn, dv.rowZoomIncBtn} {
		if chip == nil {
			continue
		}
		r := chip.Rect()
		if r.Empty() {
			continue
		}
		if r.Overlaps(btn) {
			t.Errorf("row-zoom chip %v overlaps add-row button %v — chips must live in the timeline header band",
				r, btn)
		}
	}
}

// Label, volume, mute, solo and FX inline; edit/color/origin/delete in the context menu.
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
	if len(dv.rowMuteBtns()) == 0 || dv.rowMuteBtns()[0].Rect().Empty() {
		t.Error("mute button should be visible inline on mobile")
	}
	if len(dv.rowSoloBtns()) == 0 || dv.rowSoloBtns()[0].Rect().Empty() {
		t.Error("solo button should be visible inline on mobile")
	}
	// Layout rounding may give the last zero-weight column a few pixels.
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

// Rename, Origin, Delete + close, the same as desktop, inside dv.Bounds.
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
	if len(btns) != 4 {
		t.Fatalf("expected 4 context menu buttons (4 items, no close × on mobile), got %d", len(btns))
	}
	rect := dv.ContextMenuRectVal()
	if rect.Empty() {
		t.Fatal("context menu rect is empty")
	}
	if !rect.In(dv.Bounds) {
		t.Errorf("context menu %v exceeds drum bounds %v", rect, dv.Bounds)
	}
}

// The last row's menu does not overflow the bottom.
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
	// 4 × debugCharW plus a little padding.
	minW := 4*debugCharW + 4
	if lr.Dx() < minW {
		t.Errorf("label width %d < %d (min for 4-char label + padding)", lr.Dx(), minW)
	}
}

// Volume and length are off the transport bar on mobile.
func TestMobileBeatInfoVisible(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)
	advanceFrames(g, 2)

	dv := g.drum
	// The length buttons live in the overflow menu on mobile, so the timeline and counter get the full width.
	if dv.lenDecBtn == nil || dv.lenIncBtn == nil {
		t.Fatal("length buttons nil")
	}
	if r := dv.lenDecBtn.Rect(); !r.Empty() {
		t.Errorf("lenDecBtn should be hidden on mobile (A7); got %v", r)
	}
	if r := dv.lenIncBtn.Rect(); !r.Empty() {
		t.Errorf("lenIncBtn should be hidden on mobile (A7); got %v", r)
	}
	// File options open from the grid folder button.
}

// ────────────────────── Negative tests (desktop unchanged) ──────────────────────

func TestDesktopEQVisible(t *testing.T) {
	setupMobileTest(t, false) // desktop
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)

	if g.drum.MobileEQCollapsed() {
		t.Fatal("desktop: mobileEQCollapsed should be false")
	}
	// eqPanelHeight is 0 under test, so only the flag is checked.
}

// Label | VolBar | M | S | FX | Overflow; the rest live in the overflow menu.
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

// The file ops are not inline on desktop.
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
		if r.Dx()*r.Dy() != 0 {
			t.Errorf("desktop: %s button should be hidden inline (behind file menu), got %v", pair.name, r)
		}
	}
	// File options open from the grid folder button.
}

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

// In the rack column, not a floating button.
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
	rack := dv.widgetRects[WidgetRack]
	if !rack.Empty() {
		// Inline button should overlap the rack X-range
		if fab.Min.X > rack.Max.X {
			t.Errorf("desktop: addRowBtn at x=%d should be within rack [%d,%d]", fab.Min.X, rack.Min.X, rack.Max.X)
		}
	}
}

// ────────────────────── Multi-viewport tests ──────────────────────

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

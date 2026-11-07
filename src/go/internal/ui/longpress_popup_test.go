//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

// A fresh press outside dismisses, like every pop-up; the slide-release gesture is a separate path.
func TestLongPressPopup_OutsidePressDismisses(t *testing.T) {
	g := newTestGameForUndo(t)
	n := g.tryAddNode(3, 1, model.NodeTypeRegular)
	g.updateBeatInfos()
	sx1, sy1, _, _ := g.nodeScreenRect(n)
	g.showLongPressPopup(n, int(sx1), int(sy1))
	if !g.longPressPopup {
		t.Fatal("popup should be open")
	}

	// A fresh press that lands OUTSIDE the panel dismisses it (click-outside).
	outX, outY := g.longPressPopupRect.Min.X-40, g.longPressPopupRect.Min.Y-40
	if outX < 0 {
		outX = g.longPressPopupRect.Max.X + 40
	}
	if outY < 0 {
		outY = g.longPressPopupRect.Max.Y + 40
	}
	g.leftPrev = false
	g.updateLongPressPopup(outX, outY, true) // press-down outside

	if g.longPressPopup {
		t.Fatal("press outside the panel must dismiss the popup")
	}
}

func TestLongPressPopupShowsOnNode(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	withSmallScreen(t, true)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)

	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	sx1, sy1, sx2, sy2 := g.nodeScreenRect(n)
	cx := int((sx1 + sx2) * 0.5)
	cy := int((sy1 + sy2) * 0.5)

	g.handleTouchLongPress(cx, cy)

	if !g.longPressPopup {
		t.Fatal("expected longPressPopup to be true")
	}
	if g.longPressPopupNode != n {
		t.Fatal("expected longPressPopupNode to match the pressed node")
	}
	if g.longPressPopupMove.Empty() {
		t.Fatal("expected move button rect to be non-empty")
	}
	if g.longPressPopupCopy.Empty() {
		t.Fatal("expected copy button rect to be non-empty")
	}
	if g.longPressPopupConn.Empty() {
		t.Fatal("expected connect button rect to be non-empty")
	}
	if g.longPressPopupDel.Empty() {
		t.Fatal("expected delete button rect to be non-empty")
	}
}

func TestLongPressPopupDismissOnRelease(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	withSmallScreen(t, true)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)

	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	sx1, sy1, sx2, sy2 := g.nodeScreenRect(n)
	cx := int((sx1 + sx2) * 0.5)
	cy := int((sy1 + sy2) * 0.5)
	g.handleTouchLongPress(cx, cy)

	if !g.longPressPopup {
		t.Fatal("popup should be open")
	}

	// The first frame, finger down off the buttons, sets lastHover to "".
	g.leftPrev = true
	g.updateLongPressPopup(0, 0, true)
	// Release.
	g.updateLongPressPopup(0, 0, false)

	if g.longPressPopup {
		t.Fatal("popup should be dismissed on release outside buttons")
	}
	if g.modes.Is(GridModeMove) {
		t.Fatal("moveMode should not be active")
	}
	if g.modes.Is(GridModeConnect) {
		t.Fatal("connectMode should not be active")
	}
}

func TestLongPressPopupMoveAction(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	withSmallScreen(t, true)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)

	n := g.tryAddNode(4, 0, model.NodeTypeRegular)
	sx1, sy1, sx2, sy2 := g.nodeScreenRect(n)
	cx := int((sx1 + sx2) * 0.5)
	cy := int((sy1 + sy2) * 0.5)
	g.handleTouchLongPress(cx, cy)

	// Slide to Move button center.
	mcx := (g.longPressPopupMove.Min.X + g.longPressPopupMove.Max.X) / 2
	mcy := (g.longPressPopupMove.Min.Y + g.longPressPopupMove.Max.Y) / 2

	// Hover first (finger down).
	g.leftPrev = true
	g.updateLongPressPopup(mcx, mcy, true)
	if g.longPressPopupHover != "move" {
		t.Fatalf("expected hover='move', got %q", g.longPressPopupHover)
	}

	// Release on Move button.
	g.updateLongPressPopup(mcx, mcy, false)

	if g.longPressPopup {
		t.Fatal("popup should be dismissed after action")
	}
	if !g.modes.Is(GridModeMove) {
		t.Fatal("moveMode should be true")
	}
	if g.modes.Subject() != n {
		t.Fatal("movingNode should be the long-pressed node")
	}
}

func TestLongPressPopupConnectAction(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	withSmallScreen(t, true)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)

	n := g.tryAddNode(4, 0, model.NodeTypeRegular)
	sx1, sy1, sx2, sy2 := g.nodeScreenRect(n)
	cx := int((sx1 + sx2) * 0.5)
	cy := int((sy1 + sy2) * 0.5)
	g.handleTouchLongPress(cx, cy)

	// Slide to Connect button center.
	ccx := (g.longPressPopupConn.Min.X + g.longPressPopupConn.Max.X) / 2
	ccy := (g.longPressPopupConn.Min.Y + g.longPressPopupConn.Max.Y) / 2

	g.leftPrev = true
	g.updateLongPressPopup(ccx, ccy, true) // hover
	if g.longPressPopupHover != "connect" {
		t.Fatalf("expected hover='connect', got %q", g.longPressPopupHover)
	}

	g.updateLongPressPopup(ccx, ccy, false) // release

	if g.longPressPopup {
		t.Fatal("popup should be dismissed after action")
	}
	if !g.modes.Is(GridModeConnect) {
		t.Fatal("connectMode should be true")
	}
	if g.modes.Subject() != n {
		t.Fatal("connectFromNode should be the long-pressed node")
	}
}

func TestLongPressPopupPositioning(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	withSmallScreen(t, true)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)

	// Place node at grid origin which projects to top-left area.
	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	sx1, sy1, sx2, sy2 := g.nodeScreenRect(n)
	cx := int((sx1 + sx2) * 0.5)
	cy := int((sy1 + sy2) * 0.5)
	g.handleTouchLongPress(cx, cy)

	if !g.longPressPopup {
		t.Fatal("popup should be open")
	}
	gridW := g.split.GridW(g.winW)
	gridH := g.split.GridH(g.winH)

	r := g.longPressPopupRect
	if r.Min.X < 0 || r.Min.Y < 0 {
		t.Fatalf("popup rect min out of bounds: %v", r)
	}
	if r.Max.X > gridW || r.Max.Y > gridH {
		t.Fatalf("popup rect max out of bounds: %v (grid %dx%d)", r, gridW, gridH)
	}
}

func TestLongPressPopupNotOnEmptyGrid(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	withSmallScreen(t, true)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)

	// Long-press at a position with no node.
	g.handleTouchLongPress(200, 100)

	if g.longPressPopup {
		t.Fatal("popup should not open on empty grid")
	}
}

func TestLongPressPopupShowsOnDesktop(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	// Do NOT enable small screen — desktop behavior.
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	n := g.tryAddNode(4, 0, model.NodeTypeRegular)
	nodesBefore := len(g.nodes)
	sx1, sy1, sx2, sy2 := g.nodeScreenRect(n)
	cx := int((sx1 + sx2) * 0.5)
	cy := int((sy1 + sy2) * 0.5)

	g.handleTouchLongPress(cx, cy)

	if !g.longPressPopup {
		t.Fatal("popup should open on desktop")
	}
	if len(g.nodes) != nodesBefore {
		t.Fatalf("node should NOT be deleted; nodes: %d -> %d", nodesBefore, len(g.nodes))
	}
	if g.longPressPopupMove.Empty() {
		t.Fatal("expected move button rect to be non-empty")
	}
	if g.longPressPopupCopy.Empty() {
		t.Fatal("expected copy button rect to be non-empty")
	}
	if g.longPressPopupConn.Empty() {
		t.Fatal("expected connect button rect to be non-empty")
	}
	if g.longPressPopupDel.Empty() {
		t.Fatal("expected delete button rect to be non-empty")
	}
}

func TestLongPressPopupDrumPaneUnchanged(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	withSmallScreen(t, true)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)
	advanceFrames(g, 2)

	// Ensure there is at least one row label.
	if len(g.drum.rowLabels()) == 0 {
		t.Skip("no row labels to test against")
	}
	lbl := g.drum.rowLabels()[0]
	r := lbl.Rect()
	cx := (r.Min.X + r.Max.X) / 2
	cy := (r.Min.Y + r.Max.Y) / 2

	g.handleTouchLongPress(cx, cy)

	if g.longPressPopup {
		t.Fatal("long-press popup should NOT open for drum pane")
	}
	// The context menu should have been opened by the drum pane handler.
	if !g.drum.IsContextMenuOpen() {
		t.Fatal("drum context menu should have opened")
	}
}

func TestLongPressPopupDeleteAction(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	withSmallScreen(t, true)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)

	n := g.tryAddNode(4, 0, model.NodeTypeRegular)
	nodesBefore := len(g.nodes)
	sx1, sy1, sx2, sy2 := g.nodeScreenRect(n)
	cx := int((sx1 + sx2) * 0.5)
	cy := int((sy1 + sy2) * 0.5)
	g.handleTouchLongPress(cx, cy)

	if !g.longPressPopup {
		t.Fatal("popup should be open")
	}

	// Slide to Delete button center (finger down).
	dcx := (g.longPressPopupDel.Min.X + g.longPressPopupDel.Max.X) / 2
	dcy := (g.longPressPopupDel.Min.Y + g.longPressPopupDel.Max.Y) / 2

	g.leftPrev = true
	g.updateLongPressPopup(dcx, dcy, true) // hover
	if g.longPressPopupHover != "delete" {
		t.Fatalf("expected hover='delete', got %q", g.longPressPopupHover)
	}

	// Release on Delete button.
	g.updateLongPressPopup(dcx, dcy, false)

	if g.longPressPopup {
		t.Fatal("popup should be dismissed after delete")
	}
	if len(g.nodes) != nodesBefore-1 {
		t.Fatalf("expected node to be deleted; nodes: %d -> %d", nodesBefore, len(g.nodes))
	}
}

func TestLongPressPopupNoCameraPan(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	withSmallScreen(t, true)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)

	n := g.tryAddNode(4, 0, model.NodeTypeRegular)
	sx1, sy1, sx2, sy2 := g.nodeScreenRect(n)
	cx := int((sx1 + sx2) * 0.5)
	cy := int((sy1 + sy2) * 0.5)
	g.handleTouchLongPress(cx, cy)

	if !g.longPressPopup {
		t.Fatal("popup should be open")
	}
	// The popup opens while the triggering press is still held. Calling handleTouchLongPress directly skips Update,
	// so the held state is set by hand; otherwise the drag's first frame reads as a new press and dismisses it.
	g.leftPrev = true

	origOffX := g.cam.OffsetX
	origOffY := g.cam.OffsetY

	// Simulate horizontal drag while popup is visible.
	mx := 100
	restore := SetInputForTest(
		func() (int, int) { return mx, 200 },
		func(ebiten.MouseButton) bool { return true },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return g.winW, g.winH },
	)
	defer restore()

	for i := 0; i < 10; i++ {
		mx += 20
		_ = g.Update()
	}

	if g.cam.OffsetX != origOffX || g.cam.OffsetY != origOffY {
		t.Fatalf("camera panned during popup: offset (%v,%v) -> (%v,%v)",
			origOffX, origOffY, g.cam.OffsetX, g.cam.OffsetY)
	}
}

// A lifted touch reports stale (0,0) on WASM, so the last hover from the previous frame decides.
func TestLongPressPopupReleaseActivatesButton(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	withSmallScreen(t, true)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)

	n := g.tryAddNode(4, 0, model.NodeTypeRegular)
	sx1, sy1, sx2, sy2 := g.nodeScreenRect(n)
	cx := int((sx1 + sx2) * 0.5)
	cy := int((sy1 + sy2) * 0.5)
	g.handleTouchLongPress(cx, cy)

	// Frame 1: finger on Move button (sets lastHover="move").
	mcx := (g.longPressPopupMove.Min.X + g.longPressPopupMove.Max.X) / 2
	mcy := (g.longPressPopupMove.Min.Y + g.longPressPopupMove.Max.Y) / 2
	g.leftPrev = true
	g.updateLongPressPopup(mcx, mcy, true)

	if g.longPressPopupLastHover != "move" {
		t.Fatalf("expected lastHover='move', got %q", g.longPressPopupLastHover)
	}

	g.updateLongPressPopup(0, 0, false)

	if g.longPressPopup {
		t.Fatal("popup should be dismissed")
	}
	if !g.modes.Is(GridModeMove) {
		t.Fatal("moveMode should be true despite stale release coordinates")
	}
	if g.modes.Subject() != n {
		t.Fatal("movingNode should be the long-pressed node")
	}
}

//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

// TestLongPressPopupShowsOnNode verifies that a long-press on a node on a
// small screen opens the quick-action popup with non-empty button rects.
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
	if g.longPressPopupConn.Empty() {
		t.Fatal("expected connect button rect to be non-empty")
	}
	if g.longPressPopupDel.Empty() {
		t.Fatal("expected delete button rect to be non-empty")
	}
}

// TestLongPressPopupDismissOnRelease verifies that releasing outside both
// buttons dismisses the popup without activating any mode.
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

	// Simulate: leftPrev=true, then release (left=false) at an off-popup position.
	// First frame with finger down off-buttons to set lastHover="".
	g.leftPrev = true
	g.updateLongPressPopup(0, 0, true)
	// Release.
	g.updateLongPressPopup(0, 0, false)

	if g.longPressPopup {
		t.Fatal("popup should be dismissed on release outside buttons")
	}
	if g.moveMode {
		t.Fatal("moveMode should not be active")
	}
	if g.connectMode {
		t.Fatal("connectMode should not be active")
	}
}

// TestLongPressPopupMoveAction verifies that sliding to the Move button and
// releasing enters move mode with the correct node.
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
	if !g.moveMode {
		t.Fatal("moveMode should be true")
	}
	if g.movingNode != n {
		t.Fatal("movingNode should be the long-pressed node")
	}
}

// TestLongPressPopupConnectAction verifies that sliding to the Connect button
// and releasing enters connect mode with the correct node.
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
	if !g.connectMode {
		t.Fatal("connectMode should be true")
	}
	if g.connectFromNode != n {
		t.Fatal("connectFromNode should be the long-pressed node")
	}
}

// TestLongPressPopupPositioning verifies that the popup rects are clamped
// within grid bounds even when the node is near an edge.
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

// TestLongPressPopupNotOnEmptyGrid verifies that a long-press on empty
// grid space does not open the popup.
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

// TestLongPressPopupShowsOnDesktop verifies that on a non-small screen,
// long-press now shows the popup (not deletion).
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
	if g.longPressPopupConn.Empty() {
		t.Fatal("expected connect button rect to be non-empty")
	}
	if g.longPressPopupDel.Empty() {
		t.Fatal("expected delete button rect to be non-empty")
	}
}

// TestLongPressPopupDrumPaneUnchanged verifies that a long-press on a drum
// row label on mobile still opens the context menu (not the popup).
func TestLongPressPopupDrumPaneUnchanged(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	withSmallScreen(t, true)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)
	advanceFrames(g, 2)

	// Ensure there is at least one row label.
	if len(g.drum.rowLabels) == 0 {
		t.Skip("no row labels to test against")
	}
	lbl := g.drum.rowLabels[0]
	r := lbl.Rect()
	cx := (r.Min.X + r.Max.X) / 2
	cy := (r.Min.Y + r.Max.Y) / 2

	g.handleTouchLongPress(cx, cy)

	if g.longPressPopup {
		t.Fatal("long-press popup should NOT open for drum pane")
	}
	// The context menu should have been opened by the drum pane handler.
	if !g.drum.contextMenuOpen {
		t.Fatal("drum context menu should have opened")
	}
}

// TestLongPressPopupDeleteAction verifies that sliding to the Delete button
// and releasing deletes the node and dismisses the popup.
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

// TestLongPressPopupNoCameraPan verifies that while the popup is visible,
// single-finger drag does not pan the camera.
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

// TestLongPressPopupReleaseActivatesButton verifies that button activation
// works even when release coordinates are stale (e.g. touch ended, returning
// (0,0) on WASM). The last-known hover from the previous frame is used.
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

	// Frame 2: finger lifted — coordinates become stale (0,0).
	// Current hover will be "" but lastHover should still be "move".
	g.updateLongPressPopup(0, 0, false)

	if g.longPressPopup {
		t.Fatal("popup should be dismissed")
	}
	if !g.moveMode {
		t.Fatal("moveMode should be true despite stale release coordinates")
	}
	if g.movingNode != n {
		t.Fatal("movingNode should be the long-pressed node")
	}
}

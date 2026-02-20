//go:build test

package ui

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// --- helpers ----------------------------------------------------------------

// nodeScreenCenter returns the center of a node's screen rect.
func nodeScreenCenter(g *Game, n *uiNode) (int, int) {
	x1, y1, x2, y2 := g.nodeScreenRect(n)
	return int((x1 + x2) * 0.5), int((y1 + y2) * 0.5)
}

// screenPosForGridIJ returns a screen-space coordinate that maps to grid
// subdivision (i,j) under the current camera. Similar to screenPosForGrid in
// grid_ui_interaction_test.go but duplicated here to keep the file standalone.
func screenPosForGridIJ(g *Game, i, j int) (int, int) {
	unit := g.grid.Unit()
	wx := float64(i) * unit
	wy := float64(j) * unit
	sx := int(g.cam.OffsetX + g.cam.Scale*wx)
	sy := int(g.cam.OffsetY + float64(gridTopOffset()) + g.cam.Scale*wy)
	return sx, sy
}

// --- Test: connect mode -----------------------------------------------------

func TestHandleTapInGrid_ConnectMode(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	// Create two nodes that share a coordinate (perpendicular).
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(32, 0, model.NodeTypeRegular) // same J
	g.updateBeatInfos()

	// Enter connect mode from node A.
	g.enterConnectMode(a)
	if !g.connectMode {
		t.Fatal("expected connectMode=true")
	}

	// Tap on node B to create the connection.
	bx, by := nodeScreenCenter(g, b)
	g.handleTapInGrid(bx, by)

	if g.connectMode {
		t.Fatal("expected connectMode=false after successful connect")
	}
	// Verify edge was created.
	found := false
	for _, e := range g.edges {
		if (e.A.ID == a.ID && e.B.ID == b.ID) || (e.A.ID == b.ID && e.B.ID == a.ID) {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected edge between a and b after connect mode tap")
	}
}

func TestHandleTapInGrid_ConnectMode_TapEmpty(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.updateBeatInfos()

	g.enterConnectMode(a)
	// Tap far from any node to cancel.
	emptyX, emptyY := screenPosForGridIJ(g, 100, 100)
	g.handleTapInGrid(emptyX, emptyY)

	if g.connectMode {
		t.Fatal("expected connectMode cancelled after tap on empty space")
	}
}

// --- Test: move mode --------------------------------------------------------

func TestHandleTapInGrid_MoveMode_EmptyTarget(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.updateBeatInfos()

	g.moveMode = true
	g.movingNode = n

	// Tap on empty grid position — no edges means loss=0, direct move.
	tx, ty := screenPosForGridIJ(g, 32, 0)
	g.handleTapInGrid(tx, ty)

	if g.moveMode {
		t.Fatal("expected moveMode=false after move")
	}
	if n.I != 32 || n.J != 0 {
		t.Fatalf("node at (%d,%d), want (32,0)", n.I, n.J)
	}
}

func TestHandleTapInGrid_MoveMode_OccupiedTarget(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(32, 0, model.NodeTypeRegular)
	g.updateBeatInfos()

	g.moveMode = true
	g.movingNode = a

	// Tap on position occupied by node b — should cancel.
	bx, by := screenPosForGridIJ(g, 32, 0)
	g.handleTapInGrid(bx, by)

	if g.moveMode {
		t.Fatal("expected moveMode cancelled after occupied target")
	}
	// Node a should NOT have moved.
	if a.I != 0 || a.J != 0 {
		t.Fatalf("node a at (%d,%d), want (0,0) — should not have moved", a.I, a.J)
	}
	_ = b // keep b referenced
}

func TestHandleTapInGrid_MoveMode_EdgeLoss_Confirm(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	// Create A-B edge (perpendicular). Moving A to a non-perpendicular
	// position relative to B causes edge loss > 0.
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(32, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.updateBeatInfos()

	g.moveMode = true
	g.movingNode = a

	// Move to (16,16) — neither I=16 matches B.I=32 nor J=16 matches B.J=0,
	// so loss = 1.
	tx, ty := screenPosForGridIJ(g, 16, 16)
	g.handleTapInGrid(tx, ty)

	if !g.moveConfirm {
		t.Fatal("expected moveConfirm=true due to edge loss")
	}
	if g.moveEdgeLoss != 1 {
		t.Fatalf("moveEdgeLoss=%d, want 1", g.moveEdgeLoss)
	}
}

func TestHandleTapInGrid_MoveMode_ConfirmDialog(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.updateBeatInfos()

	// Manually set up the confirm dialog state.
	g.moveMode = true
	g.movingNode = a
	g.moveConfirm = true
	g.moveConfirmI = 32
	g.moveConfirmJ = 32
	g.moveEdgeLoss = 1

	// Compute the cancel button rect (same math as the source).
	dw, dh := 280, 60
	dx := (g.split.GridW(g.winW) - dw) / 2
	dy := (g.split.GridH(g.winH) - dh) / 2
	cancelRect_x := dx + 160 + (dx+240-dx-160)/2 // center of cancel button
	cancelRect_y := dy + 30 + (dy+50-dy-30)/2

	// Tap cancel button.
	g.handleTapInGrid(cancelRect_x, cancelRect_y)

	if g.moveMode {
		t.Fatal("expected moveMode=false after cancel")
	}
	if a.I != 0 || a.J != 0 {
		t.Fatalf("node at (%d,%d), want (0,0) — cancel should not move", a.I, a.J)
	}
}

func TestHandleTapInGrid_MoveMode_ConfirmDialog_Confirm(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.updateBeatInfos()

	// Manually set up confirm dialog.
	g.moveMode = true
	g.movingNode = a
	g.moveConfirm = true
	g.moveConfirmI = 32
	g.moveConfirmJ = 0
	g.moveEdgeLoss = 1

	// Compute confirm button center.
	dw, dh := 280, 60
	dx := (g.split.GridW(g.winW) - dw) / 2
	dy := (g.split.GridH(g.winH) - dh) / 2
	confirmX := dx + 40 + (120-40)/2
	confirmY := dy + 30 + (50-30)/2

	g.handleTapInGrid(confirmX, confirmY)

	if g.moveMode {
		t.Fatal("expected moveMode=false after confirm")
	}
	if a.I != 32 || a.J != 0 {
		t.Fatalf("node at (%d,%d), want (32,0) — confirm should move", a.I, a.J)
	}
}

// --- Test: origin selection -------------------------------------------------

func TestHandleTapInGrid_OriginSelection(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, true)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	// Create a second node to use as origin for row 0.
	n2 := g.tryAddNode(32, 0, model.NodeTypeRegular)
	g.addEdge(g.nodes[0], n2)
	g.updateBeatInfos()

	// Enter origin selection for row 0.
	g.pendingStartRow = 0

	// Tap on node n2.
	nx, ny := nodeScreenCenter(g, n2)
	g.handleTapInGrid(nx, ny)

	if g.pendingStartRow != -1 {
		t.Fatalf("pendingStartRow=%d, want -1", g.pendingStartRow)
	}
	if g.drum.Rows[0].Origin != n2.ID {
		t.Fatalf("row 0 origin=%v, want %v", g.drum.Rows[0].Origin, n2.ID)
	}
	if !n2.Start {
		t.Fatal("expected n2.Start=true after origin selection")
	}
}

// --- Test: select existing node ---------------------------------------------

func TestHandleTapInGrid_SelectExistingNode(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.updateBeatInfos()
	// Close sidebar that might have been opened.
	g.sidebar.Close()
	g.sidebar.closedGuard = 0

	// Tap on the node.
	nx, ny := nodeScreenCenter(g, n)
	g.handleTapInGrid(nx, ny)

	if g.sel != n {
		t.Fatal("expected node to be selected")
	}
	if !n.Selected {
		t.Fatal("expected node.Selected=true")
	}
	if !g.sidebar.IsOpen() {
		t.Fatal("expected sidebar to be open")
	}
}

// --- Test: create new node --------------------------------------------------

func TestHandleTapInGrid_CreateNewNode(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	before := len(g.nodes)

	// Tap on empty grid space at (0,0).
	tx, ty := screenPosForGridIJ(g, 0, 0)
	g.handleTapInGrid(tx, ty)

	after := len(g.nodes)
	if after != before+1 {
		t.Fatalf("node count=%d, want %d (one new node)", after, before+1)
	}
	created := g.nodeAt(0, 0)
	if created == nil {
		t.Fatal("expected node at (0,0)")
	}
	if g.sel != created {
		t.Fatal("expected created node to be selected")
	}
	if !created.Selected {
		t.Fatal("expected created.Selected=true")
	}
}

// --- Test: sidebar close on outside tap -------------------------------------

func TestHandleTapInGrid_SidebarClose(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.updateBeatInfos()
	g.sidebar.Open(n)
	if !g.sidebar.IsOpen() {
		t.Fatal("precondition: sidebar should be open")
	}

	// Tap far outside the sidebar (and far from any node).
	farX, farY := screenPosForGridIJ(g, 100, 100)
	// Ensure this position is outside the sidebar hit area.
	if g.sidebar.Hit(farX, farY) {
		// Move even further to avoid sidebar.
		farX, farY = screenPosForGridIJ(g, 200, 200)
	}
	g.handleTapInGrid(farX, farY)

	if g.sidebar.IsOpen() {
		t.Fatal("expected sidebar to be closed after tap outside")
	}
}

// --- Test: closed guard suppresses new node creation ------------------------

func TestHandleTapInGrid_ClosedGuard(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	// Manually set closedGuard to simulate recently closed sidebar.
	g.sidebar.closedGuard = 2

	before := len(g.nodes)
	tx, ty := screenPosForGridIJ(g, 0, 0)
	g.handleTapInGrid(tx, ty)

	if len(g.nodes) != before {
		t.Fatalf("node count=%d, want %d (guard should suppress creation)", len(g.nodes), before)
	}
}

// --- Test: pinch zoom -------------------------------------------------------

func TestHandleTouchPinch(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	// Ensure pinch is in the grid pane.
	cx, cy := g.winW/2, g.split.Y/2
	if cy < gridTopOffset() {
		cy = gridTopOffset() + 10
	}

	origScale := g.cam.Scale

	// First call: records baseline.
	g.handleTouchPinch(cx, cy, 1.0)
	if g.pinchBaseScale != origScale {
		t.Fatalf("pinchBaseScale=%v, want %v", g.pinchBaseScale, origScale)
	}
	if g.pinchBaseGestureScale != 1.0 {
		t.Fatalf("pinchBaseGestureScale=%v, want 1.0", g.pinchBaseGestureScale)
	}
	// Scale should NOT change on first call.
	if g.cam.Scale != origScale {
		t.Fatalf("cam.Scale=%v, want %v (unchanged on first pinch)", g.cam.Scale, origScale)
	}

	// Second call: zoom in 2x.
	g.handleTouchPinch(cx, cy, 2.0)
	want := origScale * 2.0
	if math.Abs(g.cam.Scale-want) > 0.01 {
		t.Fatalf("cam.Scale=%v, want ~%v after 2x pinch", g.cam.Scale, want)
	}
}

func TestHandleTouchPinch_ClampMin(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	cx, cy := g.winW/2, g.split.Y/2
	if cy < gridTopOffset() {
		cy = gridTopOffset() + 10
	}

	g.handleTouchPinch(cx, cy, 1.0) // baseline
	// Pinch to extreme zoom-out: gesture scale very small.
	g.handleTouchPinch(cx, cy, 0.001)
	if g.cam.Scale < 0.1 {
		t.Fatalf("cam.Scale=%v, want >= 0.1 (clamped)", g.cam.Scale)
	}
}

func TestHandleTouchPinch_ClampMax(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	cx, cy := g.winW/2, g.split.Y/2
	if cy < gridTopOffset() {
		cy = gridTopOffset() + 10
	}

	g.handleTouchPinch(cx, cy, 1.0)
	// Extreme zoom-in.
	g.handleTouchPinch(cx, cy, 1000.0)
	if g.cam.Scale > 10.0 {
		t.Fatalf("cam.Scale=%v, want <= 10.0 (clamped)", g.cam.Scale)
	}
}

func TestHandleTouchPinch_OutsideGrid(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	origScale := g.cam.Scale

	// Pinch above the grid top offset (y < gridTopOffset).
	g.handleTouchPinch(g.winW/2, gridTopOffset()-1, 1.0)
	if g.cam.Scale != origScale {
		t.Fatal("expected no-op when pinch is above grid top offset")
	}

	// Pinch below splitter (not in grid pane).
	g.handleTouchPinch(g.winW/2, g.split.Y+50, 1.0)
	if g.cam.Scale != origScale {
		t.Fatal("expected no-op when pinch is below grid pane")
	}
}

// --- Test: two-finger pan ---------------------------------------------------

func TestHandleTouchTwoFingerPan(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	origX := g.cam.OffsetX
	origY := g.cam.OffsetY

	g.handleTouchTwoFingerPan(10, -5)

	// OffsetX and OffsetY are snapped to integer pixels.
	wantX := math.Round(origX + 10)
	wantY := math.Round(origY - 5)
	if g.cam.OffsetX != wantX {
		t.Fatalf("cam.OffsetX=%v, want %v", g.cam.OffsetX, wantX)
	}
	if g.cam.OffsetY != wantY {
		t.Fatalf("cam.OffsetY=%v, want %v", g.cam.OffsetY, wantY)
	}
}

func TestHandleTouchTwoFingerPan_Accumulates(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	origX := g.cam.OffsetX
	origY := g.cam.OffsetY

	g.handleTouchTwoFingerPan(5, 3)
	g.handleTouchTwoFingerPan(5, 3)

	wantX := math.Round(origX + 10)
	wantY := math.Round(origY + 6)
	if g.cam.OffsetX != wantX {
		t.Fatalf("cam.OffsetX=%v, want %v", g.cam.OffsetX, wantX)
	}
	if g.cam.OffsetY != wantY {
		t.Fatalf("cam.OffsetY=%v, want %v", g.cam.OffsetY, wantY)
	}
}

// --- Test: long press on grid node ------------------------------------------

func TestHandleTouchLongPress_GridNode(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.updateBeatInfos()

	nx, ny := nodeScreenCenter(g, n)
	g.handleTouchLongPress(nx, ny)

	if !g.longPressPopup {
		t.Fatal("expected longPressPopup=true after long press on node")
	}
	if g.longPressPopupNode != n {
		t.Fatal("expected longPressPopupNode to be the pressed node")
	}
}

func TestHandleTouchLongPress_OutsideGrid(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	// Long press above gridTopOffset — should be a no-op.
	g.handleTouchLongPress(g.winW/2, gridTopOffset()-1)
	if g.longPressPopup {
		t.Fatal("expected no popup when long press is above grid")
	}

	// Long press below splitter (in drum pane) — desktop ignores (not mobile).
	g.handleTouchLongPress(g.winW/2, g.split.Y+50)
	if g.longPressPopup {
		t.Fatal("expected no popup when long press is below grid pane")
	}
}

func TestHandleTouchLongPress_EmptySpace(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	// Long press on empty grid space (no node) — should not open popup.
	emptyX, emptyY := screenPosForGridIJ(g, 100, 100)
	// Ensure it's in the grid pane.
	if emptyY >= g.split.Y || emptyY < gridTopOffset() {
		emptyX, emptyY = g.winW/2, (gridTopOffset()+g.split.Y)/2
	}
	g.handleTouchLongPress(emptyX, emptyY)

	if g.longPressPopup {
		t.Fatal("expected no popup when long press is on empty grid space")
	}
}

// --- Test: IsTouchActive (simple) -------------------------------------------

func TestIsTouchActive_NoTouches(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	// Default state: no touches active.
	if g.IsTouchActive() {
		t.Fatal("expected IsTouchActive()=false with no touches")
	}
}

// TestMobileGridCapturingFalseDuringGridTouch checks that Capturing() returns
// false when a touch is on the grid pane (not the drum pane). This is a targeted
// diagnostic for the one-finger grid panning regression.
func TestMobileGridCapturingFalseDuringGridTouch(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)

	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false; activeProfile = nil })

	globalTouchState.Reset()
	t.Cleanup(func() { globalTouchState.Reset(); resetTouchOverride() })

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(360, 640)

	// Override Ebiten cursor fallback.
	oldEbCursor := _ebCursorPosition
	_ebCursorPosition = func() (int, int) { return 0, 0 }
	defer func() { _ebCursorPosition = oldEbCursor }()

	mock := newMockTouchState()
	restoreTouch := SetTouchForTest(mock.TouchIDs, mock.TouchPosition)
	defer restoreTouch()

	// Let layout settle.
	advanceFrames(g, 2)

	gridCenterX := 180
	gridCenterY := g.split.Y / 2
	if gridCenterY < 10 {
		gridCenterY = 10
	}

	// Touch down on grid.
	mock.addTouch(1, gridCenterX, gridCenterY)
	_ = g.Update()

	// Check Capturing() state during a grid touch.
	if g.drum.Capturing() {
		t.Errorf("Capturing() should be false during grid touch at (%d,%d), drum.Bounds=%v",
			gridCenterX, gridCenterY, g.drum.Bounds)
		t.Logf("  mouseDownInBounds=%v", g.drum.mouseDownInBounds)
		t.Logf("  anyDragActive=%v", g.drum.anyDragActive())
		t.Logf("  rowScroll().TouchActive=%v", g.drum.rowScroll().TouchActive())
		t.Logf("  anyDropdownOpen=%v", g.drum.anyDropdownOpen())
		if g.drum.tree != nil {
			t.Logf("  portal.IsOpen=%v stackLen=%d", g.drum.tree.Portal().IsOpen(), g.drum.tree.Portal().StackLen())
		}
		t.Logf("  touchOverrideActive=%v touchOverrideX=%d touchOverrideY=%d",
			touchOverrideActive, touchOverrideX, touchOverrideY)
	}

	mock.removeTouch(1)
	advanceFrames(g, 2)
}

// TestMobileGridPanAfterPortalClose verifies that one-finger grid panning
// works correctly after a portal overlay has been opened and closed. This
// tests for stale portal entries blocking panOK.
func TestMobileGridPanAfterPortalClose(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, true)

	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false; activeProfile = nil })

	globalTouchState.Reset()
	t.Cleanup(func() { globalTouchState.Reset(); resetTouchOverride() })

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(360, 640)

	oldEbCursor := _ebCursorPosition
	_ebCursorPosition = func() (int, int) { return 0, 0 }
	defer func() { _ebCursorPosition = oldEbCursor }()

	mock := newMockTouchState()
	restoreTouch := SetTouchForTest(mock.TouchIDs, mock.TouchPosition)
	defer restoreTouch()

	advanceFrames(g, 2)

	// Simulate opening and closing a portal (volume popup).
	if g.drum.tree != nil && g.drum.tree.Portal() != nil {
		g.drum.openVolPopupPortal()
		advanceFrames(g, 2)

		if !g.drum.anyDropdownOpen() {
			t.Log("Portal did not open (openVolPopupPortal may be a no-op)")
		} else {
			// Close it.
			g.drum.CloseAllPopups()
			advanceFrames(g, 2)

			if g.drum.anyDropdownOpen() {
				t.Errorf("Portal still open after CloseAllPopups; portal stackLen=%d",
					g.drum.tree.Portal().StackLen())
			}
		}
	}

	// Now test grid panning.
	initialNodes := len(g.nodes)
	initialOffX := g.cam.OffsetX

	gridCenterX := 180
	gridCenterY := g.split.Y / 2
	if gridCenterY < 10 {
		gridCenterY = 10
	}

	// Touch and drag on grid.
	mock.addTouch(1, gridCenterX, gridCenterY)
	advanceFrames(g, 1)

	for frame := 0; frame < 5; frame++ {
		dx := (frame + 1) * 10
		mock.moveTouch(1, gridCenterX+dx, gridCenterY)
		advanceFrames(g, 1)
	}

	mock.removeTouch(1)
	advanceFrames(g, 3)

	if len(g.nodes) != initialNodes {
		t.Errorf("grid drag created %d nodes after portal close", len(g.nodes)-initialNodes)
	}
	if g.cam.OffsetX == initialOffX {
		t.Errorf("camera did not pan after portal close")
	}
}

// TestMobileGridPanAfterDrumTouch verifies that grid panning works after
// a prior touch on the drum pane. The InputDispatcher capture from the
// drum touch must be released before the grid touch starts.
func TestMobileGridPanAfterDrumTouch(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, true) // need a start node for rows

	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false; activeProfile = nil })

	globalTouchState.Reset()
	t.Cleanup(func() { globalTouchState.Reset(); resetTouchOverride() })

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(360, 640)

	oldEbCursor := _ebCursorPosition
	_ebCursorPosition = func() (int, int) { return 0, 0 }
	defer func() { _ebCursorPosition = oldEbCursor }()

	mock := newMockTouchState()
	restoreTouch := SetTouchForTest(mock.TouchIDs, mock.TouchPosition)
	defer restoreTouch()

	advanceFrames(g, 2) // let layout settle

	drumBounds := g.drum.Bounds

	// Step 1: Touch the drum pane briefly (tap).
	drumTouchX := drumBounds.Min.X + drumBounds.Dx()/2
	drumTouchY := drumBounds.Min.Y + drumBounds.Dy()/2

	mock.addTouch(1, drumTouchX, drumTouchY)
	advanceFrames(g, 2) // touch down

	mock.removeTouch(1)
	advanceFrames(g, 5) // release + gesture processing + cooldown

	// Verify clean state after drum touch release.
	if g.drum.Capturing() {
		t.Errorf("Capturing() should be false after drum touch release; "+
			"anyDragActive=%v rowScroll.TouchActive=%v anyDropdownOpen=%v",
			g.drum.anyDragActive(), g.drum.rowScroll().TouchActive(), g.drum.anyDropdownOpen())
	}

	// Step 2: Now drag on the grid pane.
	initialNodes := len(g.nodes)
	initialOffX := g.cam.OffsetX

	gridCenterX := 180
	gridCenterY := g.split.Y / 2
	if gridCenterY < 10 {
		gridCenterY = 10
	}

	mock.addTouch(1, gridCenterX, gridCenterY)
	advanceFrames(g, 1)

	for frame := 0; frame < 5; frame++ {
		dx := (frame + 1) * 10
		mock.moveTouch(1, gridCenterX+dx, gridCenterY)
		advanceFrames(g, 1)
	}

	mock.removeTouch(1)
	advanceFrames(g, 3)

	if len(g.nodes) != initialNodes {
		t.Errorf("grid drag after drum touch created %d nodes", len(g.nodes)-initialNodes)
	}
	if g.cam.OffsetX == initialOffX {
		t.Errorf("camera did not pan after drum touch + grid drag")
	}
}

// TestMobileOneFingerGridDrag verifies that a single-finger drag on the
// grid pane pans the camera instead of creating nodes. This is a regression
// test for a bug where the portal tree refactor caused g.drum.Capturing()
// to return true during grid touches, blocking panOK and preventing panning.
func TestMobileOneFingerGridDrag(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)

	// Force mobile profile.
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false; activeProfile = nil })

	// Reset global touch state to avoid stale cooldowns from prior tests.
	globalTouchState.Reset()
	t.Cleanup(func() { globalTouchState.Reset(); resetTouchOverride() })

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(360, 640) // mobile portrait

	// Override Ebiten cursor fallback so cursorPosition() returns a valid
	// position when touch override is inactive (e.g., between touches).
	oldEbCursor := _ebCursorPosition
	_ebCursorPosition = func() (int, int) { return 0, 0 }
	defer func() { _ebCursorPosition = oldEbCursor }()

	// Record initial state.
	initialNodes := len(g.nodes)

	// Find a screen position in the grid pane (above the splitter).
	gridCenterX := 180
	gridCenterY := g.split.Y / 2
	if gridCenterY < 10 {
		gridCenterY = 10
	}

	// Verify the point is actually in the grid pane.
	if !g.split.InGridPane(gridCenterX, gridCenterY) {
		t.Fatalf("test point (%d,%d) not in grid pane (split.Y=%d)",
			gridCenterX, gridCenterY, g.split.Y)
	}
	if pt(gridCenterX, gridCenterY, g.drum.Bounds) {
		t.Fatalf("test point (%d,%d) is inside drum bounds %v",
			gridCenterX, gridCenterY, g.drum.Bounds)
	}

	// Set up mock touch.
	mock := newMockTouchState()
	restoreTouch := SetTouchForTest(mock.TouchIDs, mock.TouchPosition)
	defer restoreTouch()

	// Save camera position after initial setup frames.
	advanceFrames(g, 2) // let layout settle
	initialOffX := g.cam.OffsetX
	initialOffY := g.cam.OffsetY

	// Frame 1: finger down at grid center.
	mock.addTouch(1, gridCenterX, gridCenterY)
	advanceFrames(g, 1)

	// Frames 2-6: drag finger 50px to the right (10px/frame for 5 frames).
	// This ensures movement > tapMaxMovePx (10px) so it's NOT classified as tap.
	for frame := 0; frame < 5; frame++ {
		dx := (frame + 1) * 10
		newX := gridCenterX + dx
		mock.moveTouch(1, newX, gridCenterY)
		advanceFrames(g, 1)
	}

	// Frame 7: release finger.
	mock.removeTouch(1)
	advanceFrames(g, 3) // extra frames for gesture processing

	// Assert: NO new nodes were created.
	if len(g.nodes) != initialNodes {
		t.Errorf("one-finger grid drag created %d nodes (had %d, now %d); "+
			"expected panning, not node creation",
			len(g.nodes)-initialNodes, initialNodes, len(g.nodes))
	}

	// Assert: camera offset changed (panning occurred).
	if g.cam.OffsetX == initialOffX && g.cam.OffsetY == initialOffY {
		t.Errorf("camera did not move after one-finger drag on grid "+
			"(offset unchanged at (%.1f, %.1f)); panOK was likely false",
			initialOffX, initialOffY)
	}
}

//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

// On mobile one touch spans frames: the override press sets pendingClick, the gesture tap closes the popup,
// and the release would then add a node from the stale pendingClick; the closed guard stops it.
func TestPopupCloseNoNodeCreation(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	// Add a node and open its popup.
	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	if n == nil {
		t.Fatal("failed to add node at (0,0)")
	}
	g.sel = n
	n.Selected = true
	g.sidebar.Open(n)
	g.sidebar.layout()

	// Far from the node at (0,0), outside the menu.
	tapX, tapY := g.winW-20, g.split.GridH(g.winH)/2
	if g.menuHit(tapX, tapY) {
		t.Fatal("test position should be outside popup panel")
	}

	nodesBefore := len(g.nodes)

	// handleEditor sees the press edge and sets pendingClick.
	restore := SetInputForTest(
		func() (int, int) { return tapX, tapY },
		func(btn ebiten.MouseButton) bool { return btn == ebiten.MouseButtonLeft },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return g.winW, g.winH },
	)
	g.handleEditor()
	restore()

	if !g.pendingClick {
		t.Fatal("expected pendingClick=true after press frame")
	}

	// --- Frame N+1: gesture tap fires at same position, closes popup ---
	g.handleTapInGrid(tapX, tapY)

	if g.sidebar.IsOpen() {
		t.Fatal("popup should be closed after handleTapInGrid")
	}
	if g.sidebar.closedGuard == 0 {
		t.Fatal("nodeMenuClosedGuard should be set after popup close")
	}

	// Without the guard the release handler would call tryAddNode.
	restore = SetInputForTest(
		func() (int, int) { return tapX, tapY },
		func(btn ebiten.MouseButton) bool { return false }, // released
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return g.winW, g.winH },
	)
	g.handleEditor()
	restore()

	if len(g.nodes) != nodesBefore {
		t.Fatalf("expected %d nodes after release, got %d — node created under closed popup", nodesBefore, len(g.nodes))
	}
	if g.pendingClick {
		t.Fatal("pendingClick should be cleared by the guard")
	}
}

// The close button sets the guard through handleNodeMenuButtons.
func TestPopupCloseButtonNoNodeCreation(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	// Add a node and open its popup.
	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	if n == nil {
		t.Fatal("failed to add node at (0,0)")
	}
	g.sel = n
	n.Selected = true
	g.sidebar.Open(n)
	g.sidebar.layout()

	// Verify close button exists.
	r, ok := g.sidebar.rects[sbClose]
	if !ok || r.Empty() {
		t.Fatal("close rect missing or empty")
	}
	cx := (r.Min.X + r.Max.X) / 2
	cy := (r.Min.Y + r.Max.Y) / 2

	nodesBefore := len(g.nodes)

	// As the InputDispatcher routes it.
	g.sidebar.HandleInput(cx, cy, true)

	if g.sidebar.IsOpen() {
		t.Fatal("popup should be closed after close button press")
	}
	if g.sidebar.closedGuard == 0 {
		t.Fatal("nodeMenuClosedGuard should be set after popup close")
	}

	// --- Frame N+1: gesture tap at same position ---
	g.handleTapInGrid(cx, cy)

	if len(g.nodes) != nodesBefore {
		t.Fatalf("expected %d nodes after tap, got %d — node created under close button", nodesBefore, len(g.nodes))
	}
}

// Even a quick tap lasts ~3 frames: the guard is frozen while a touch is down, or it runs out before the
// GestureTap arrives and a node is added.
func TestPopupCloseButtonMultiFrameTouch(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	globalTouchState.Reset() // Prevent stale cooldown from prior touch tests
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	// Add a node and open its popup.
	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	if n == nil {
		t.Fatal("failed to add node at (0,0)")
	}
	g.sel = n
	n.Selected = true
	g.sidebar.Open(n)
	g.sidebar.layout()

	// Get close button position.
	r, ok := g.sidebar.rects[sbClose]
	if !ok || r.Empty() {
		t.Fatal("close rect missing or empty")
	}
	cx := (r.Min.X + r.Max.X) / 2
	cy := (r.Min.Y + r.Max.Y) / 2

	nodesBefore := len(g.nodes)

	// --- Frame N: Touch starts. Close button fires via sidebar HandleInput → guard=2 ---
	restore := SetInputForTest(
		func() (int, int) { return cx, cy },
		func(btn ebiten.MouseButton) bool { return btn == ebiten.MouseButtonLeft },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return g.winW, g.winH },
	)
	g.sidebar.HandleInput(cx, cy, true)
	restore()

	if g.sidebar.IsOpen() {
		t.Fatal("popup should be closed after close button press")
	}
	if g.sidebar.closedGuard != 2 {
		t.Fatalf("expected guard=2, got %d", g.sidebar.closedGuard)
	}

	// ActiveTouchCount() returns 1.
	mock := newMockTouchState()
	mock.addTouch(1, cx, cy)
	restoreTouch := SetTouchForTest(mock.TouchIDs, mock.TouchPosition)
	defer restoreTouch()

	// Feed the touch into globalTouchState so ActiveTouchCount() reflects it.
	globalTouchState.Update()
	if globalTouchState.ActiveTouchCount() != 1 {
		t.Fatalf("expected 1 active touch, got %d", globalTouchState.ActiveTouchCount())
	}

	// Frozen while a touch is down.
	if g.sidebar.closedGuard > 0 && globalTouchState.ActiveTouchCount() == 0 {
		g.sidebar.closedGuard--
	}
	if g.sidebar.closedGuard != 2 {
		t.Fatalf("guard should stay at 2 while touch active, got %d", g.sidebar.closedGuard)
	}

	// --- Frame N+2: Guard decrement runs again (touch still held) ---
	if g.sidebar.closedGuard > 0 && globalTouchState.ActiveTouchCount() == 0 {
		g.sidebar.closedGuard--
	}
	if g.sidebar.closedGuard != 2 {
		t.Fatalf("guard should still be 2 while touch active, got %d", g.sidebar.closedGuard)
	}

	// --- Touch ends: remove from mock, Update() fires GestureTap ---
	mock.removeTouch(1)
	gesture := globalTouchState.Update()

	// This frame's decrement already ran at the top of Update.
	if g.sidebar.closedGuard != 2 {
		t.Fatalf("guard should be 2 when tap fires, got %d", g.sidebar.closedGuard)
	}

	// GestureTap should have fired.
	if gesture == nil || gesture.Kind != GestureTap {
		t.Fatal("expected GestureTap after touch release")
	}

	// handleTapInGrid: guard=2 > 0 → blocked, no node created.
	g.handleTapInGrid(cx, cy)
	if len(g.nodes) != nodesBefore {
		t.Fatalf("expected %d nodes, got %d — node created under closed popup", nodesBefore, len(g.nodes))
	}

	// --- After tap: guard should decrement normally with no touches ---
	if globalTouchState.ActiveTouchCount() != 0 {
		t.Fatal("expected 0 active touches after release")
	}
	// Simulate 2 frames of decrement with no touches.
	if g.sidebar.closedGuard > 0 && globalTouchState.ActiveTouchCount() == 0 {
		g.sidebar.closedGuard--
	}
	if g.sidebar.closedGuard != 1 {
		t.Fatalf("expected guard=1 after first decay, got %d", g.sidebar.closedGuard)
	}
	if g.sidebar.closedGuard > 0 && globalTouchState.ActiveTouchCount() == 0 {
		g.sidebar.closedGuard--
	}
	if g.sidebar.closedGuard != 0 {
		t.Fatalf("expected guard=0 after second decay, got %d", g.sidebar.closedGuard)
	}
}

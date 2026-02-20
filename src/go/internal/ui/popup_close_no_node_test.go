//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

// TestPopupCloseNoNodeCreation verifies that tapping outside the node popup
// to close it does NOT create a node underneath. On mobile, the same touch
// interaction spans multiple frames through two code paths:
//
//	Frame N:   touch-to-mouse override → handleEditor → sets pendingClick=true
//	Frame N+1: gesture tap → handleTapInGrid → closes popup, sets guard
//	Frame N+2: touch ended, handleEditor → release handler with stale pendingClick
//
// The nodeMenuClosedGuard in handleEditor prevents the release handler from
// firing tryAddNode with stale state.
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

	// Find a screen position outside the popup panel to simulate the tap.
	// Use a position far from the node at (0,0) so it's outside the menu.
	tapX, tapY := g.winW-20, g.split.GridH(g.winH)/2
	if g.menuHit(tapX, tapY) {
		t.Fatal("test position should be outside popup panel")
	}

	nodesBefore := len(g.nodes)

	// --- Frame N: touch press at outside-popup position via mouse override ---
	// handleEditor sees left=true, !leftPrev → sets pendingClick=true.
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

	// --- Frame N+2: touch released, handleEditor runs with left=false ---
	// Without the guard, this would enter the release handler (left=false,
	// leftPrev=true, pendingClick=true) and call tryAddNode.
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

// TestPopupCloseButtonNoNodeCreation verifies the same scenario but when
// the close button is tapped directly — the close button fires via
// handleNodeMenuButtons, setting the guard.
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
	r, ok := g.sidebar.rects["close"]
	if !ok || r.Empty() {
		t.Fatal("close rect missing or empty")
	}
	cx := (r.Min.X + r.Max.X) / 2
	cy := (r.Min.Y + r.Max.Y) / 2

	nodesBefore := len(g.nodes)

	// --- Frame N: press on close button via sidebar HandleInput (as the
	// InputDispatcher would route it in the real Update path) ---
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

// TestPopupCloseButtonMultiFrameTouch reproduces the exact bug scenario where
// a mobile touch lasting 3+ frames (very common — even a quick tap is ~50ms /
// 3 frames at 60fps) exhausts the guard before the GestureTap fires.
//
// Timeline without the fix:
//
//	Frame N:   Touch starts. handleEditor → close button fires → guard=2.
//	Frame N+1: guard decremented 2→1. Touch still held. handleEditor → guard>0 → blocked (ok).
//	Frame N+2: guard decremented 1→0. Touch ends → GestureTap detected.
//	           handleTapInGrid: guard=0 → NOT blocked → tryAddNode() → BUG!
//
// The fix freezes the guard while ActiveTouchCount() > 0, so it remains at 2
// until the touch fully ends and the GestureTap can be blocked.
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
	r, ok := g.sidebar.rects["close"]
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

	// Simulate a touch being held via mock touch state. This makes
	// globalTouchState.ActiveTouchCount() return 1.
	mock := newMockTouchState()
	mock.addTouch(1, cx, cy)
	restoreTouch := SetTouchForTest(mock.TouchIDs, mock.TouchPosition)
	defer restoreTouch()

	// Feed the touch into globalTouchState so ActiveTouchCount() reflects it.
	globalTouchState.Update()
	if globalTouchState.ActiveTouchCount() != 1 {
		t.Fatalf("expected 1 active touch, got %d", globalTouchState.ActiveTouchCount())
	}

	// --- Frame N+1: Guard decrement runs (touch still held) ---
	// With the fix, guard should NOT decrement because ActiveTouchCount() > 0.
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

	// ActiveTouchCount is now 0, but the guard decrement for this frame
	// already ran (at top of Update loop). The guard is still 2.
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

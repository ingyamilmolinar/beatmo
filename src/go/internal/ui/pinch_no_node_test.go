//go:build test

package ui

import (
	"testing"
)

// The trailing single finger would activate the touch-to-mouse override and read as a fresh press.
func TestPinchEndDoesNotCreateNode(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	mock := newMockTouchState()
	restore := SetTouchForTest(mock.TouchIDs, mock.TouchPosition)
	defer restore()
	defer resetTouchOverride()

	nodesBefore := len(g.nodes)

	// Two fingers down (pinch gesture in grid area).
	mock.addTouch(1, 200, 100)
	mock.addTouch(2, 400, 100)
	advanceFrames(g, 1) // initializes two-finger tracking

	// Move fingers apart to trigger pinch.
	mock.moveTouch(1, 180, 100)
	mock.moveTouch(2, 420, 100)
	advanceFrames(g, 3)

	// Asymmetric lift: finger 2 lifts, finger 1 remains.
	mock.removeTouch(2)
	advanceFrames(g, 3) // cooldown should suppress override

	// Remaining finger lifts.
	mock.removeTouch(1)
	advanceFrames(g, 3)

	if len(g.nodes) != nodesBefore {
		t.Errorf("Expected %d nodes (unchanged), got %d — pinch end created a node", nodesBefore, len(g.nodes))
	}
}

func TestPinchEndSymmetricDoesNotCreateNode(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	mock := newMockTouchState()
	restore := SetTouchForTest(mock.TouchIDs, mock.TouchPosition)
	defer restore()
	defer resetTouchOverride()

	nodesBefore := len(g.nodes)

	// Two fingers down.
	mock.addTouch(1, 200, 100)
	mock.addTouch(2, 400, 100)
	advanceFrames(g, 1)

	// Pinch motion.
	mock.moveTouch(1, 180, 100)
	mock.moveTouch(2, 420, 100)
	advanceFrames(g, 3)

	// Both fingers lift simultaneously.
	mock.removeTouch(1)
	mock.removeTouch(2)
	advanceFrames(g, 3)

	if len(g.nodes) != nodesBefore {
		t.Errorf("Expected %d nodes (unchanged), got %d — symmetric pinch end created a node", nodesBefore, len(g.nodes))
	}
}

func TestTwoFingerPanEndDoesNotCreateNode(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	mock := newMockTouchState()
	restore := SetTouchForTest(mock.TouchIDs, mock.TouchPosition)
	defer restore()
	defer resetTouchOverride()

	nodesBefore := len(g.nodes)

	// Two fingers down.
	mock.addTouch(1, 200, 100)
	mock.addTouch(2, 300, 100)
	advanceFrames(g, 1)

	// Two-finger pan (both fingers move in the same direction, same distance).
	mock.moveTouch(1, 210, 110)
	mock.moveTouch(2, 310, 110)
	advanceFrames(g, 3)

	// Asymmetric lift.
	mock.removeTouch(2)
	advanceFrames(g, 3)

	mock.removeTouch(1)
	advanceFrames(g, 3)

	if len(g.nodes) != nodesBefore {
		t.Errorf("Expected %d nodes (unchanged), got %d — two-finger pan end created a node", nodesBefore, len(g.nodes))
	}
}

// The first finger alone maps to a mouse press; the second arriving next frame drops the override, which reads as a
// release with a pending click unless handleEditor's multi-touch guard holds.
func TestPinchStartStaggeredDoesNotCreateNode(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	mock := newMockTouchState()
	restore := SetTouchForTest(mock.TouchIDs, mock.TouchPosition)
	defer restore()
	defer resetTouchOverride()

	// When the override deactivates, cursorPosition() must still land in the grid, as Ebiten's (0,0) does on mobile.
	oldEbCursor := _ebCursorPosition
	_ebCursorPosition = func() (int, int) { return 200, 100 }
	defer func() { _ebCursorPosition = oldEbCursor }()

	nodesBefore := len(g.nodes)

	// Frame N: first finger lands at a grid position.
	mock.addTouch(1, 200, 100)
	advanceFrames(g, 1)
	// The override maps this finger to left=true, and handleEditor sets pendingClick.

	// Frame N+1: second finger arrives (staggered pinch start).
	mock.addTouch(2, 400, 100)
	advanceFrames(g, 1)
	// Two touches drop the override: left becomes false with pendingClick set.

	// Continue the pinch gesture for a few frames.
	mock.moveTouch(1, 180, 100)
	mock.moveTouch(2, 420, 100)
	advanceFrames(g, 3)

	// Lift both fingers.
	mock.removeTouch(1)
	mock.removeTouch(2)
	advanceFrames(g, 3)

	if len(g.nodes) != nodesBefore {
		t.Errorf("Expected %d nodes (unchanged), got %d — staggered pinch start created a spurious node", nodesBefore, len(g.nodes))
	}
}

func TestMultiTouchCooldownExpires(t *testing.T) {
	ts := NewTouchState()
	mock := newMockTouchState()
	restore := SetTouchForTest(mock.TouchIDs, mock.TouchPosition)
	defer restore()

	// Two fingers down → initialize gesture tracking.
	mock.addTouch(1, 100, 100)
	mock.addTouch(2, 200, 100)
	ts.Update()

	// Move to trigger pinch detection.
	mock.moveTouch(1, 80, 100)
	mock.moveTouch(2, 220, 100)
	ts.Update()

	if !ts.twoFingerStarted {
		t.Fatal("Expected twoFingerStarted=true after two-finger gesture")
	}

	// Lift one finger → cooldown should start.
	mock.removeTouch(2)
	ts.Update()

	if !ts.RecentMultiTouch() {
		t.Fatal("Expected RecentMultiTouch()=true immediately after finger lift")
	}
	if ts.multiTouchCooldown != multiTouchCooldownFrames {
		t.Errorf("Expected cooldown=%d, got %d", multiTouchCooldownFrames, ts.multiTouchCooldown)
	}

	// Advance frames — cooldown should decrement.
	mock.removeTouch(1)
	for i := 0; i < multiTouchCooldownFrames; i++ {
		if !ts.RecentMultiTouch() {
			t.Fatalf("Cooldown expired too early at frame %d", i)
		}
		ts.Update()
	}

	// After exactly multiTouchCooldownFrames updates, cooldown should be 0.
	if ts.RecentMultiTouch() {
		t.Errorf("Expected RecentMultiTouch()=false after %d frames, cooldown=%d", multiTouchCooldownFrames, ts.multiTouchCooldown)
	}
}

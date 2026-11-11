//go:build test

package ui

import (
	"testing"
)

// TestPinchEndDoesNotCreateNode verifies that an asymmetric two-finger lift
// (one finger lifts before the other) after a pinch gesture does not create
// a node. This was a bug where the trailing single finger activated the
// touch-to-mouse override, causing the editor to see a fresh left-press
// transition and create a node.
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

// TestPinchEndSymmetricDoesNotCreateNode verifies that both fingers lifting
// on the same frame after a pinch does not create a node.
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

// TestTwoFingerPanEndDoesNotCreateNode verifies that an asymmetric lift
// after a two-finger pan gesture does not create a node.
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

// TestPinchStartStaggeredDoesNotCreateNode verifies that when two fingers
// land on separate frames (staggered arrival, as happens on real mobile),
// no node is created. The first finger arrives alone and activates the
// touch-to-mouse override (left=true), then the second finger arrives on
// the next frame, deactivating the override (left=false). Without the
// multi-touch guard in handleEditor(), this transition looks like a
// mouse-release with pendingClick=true and creates a spurious node.
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

	// Override the raw Ebiten cursor fallback so that when the touch override
	// deactivates (2 fingers), cursorPosition() still returns a valid grid
	// position (y >= gridTopOffset). On real mobile, gridTopOffset()=0 and
	// Ebiten returns (0,0) which is in the grid area. Without this, the
	// desktop fallback returns (0,0) from the stub which also happens to be
	// in the grid — but let's be explicit.
	oldEbCursor := _ebCursorPosition
	_ebCursorPosition = func() (int, int) { return 200, 100 }
	defer func() { _ebCursorPosition = oldEbCursor }()

	nodesBefore := len(g.nodes)

	// Frame N: first finger lands at a grid position.
	mock.addTouch(1, 200, 100)
	advanceFrames(g, 1)
	// The touch override maps this single finger to left=true.
	// handleEditor() sets pendingClick=true.

	// Frame N+1: second finger arrives (staggered pinch start).
	mock.addTouch(2, 400, 100)
	advanceFrames(g, 1)
	// The override deactivates (ActiveTouchCount=2), so left becomes false.
	// Without the fix, handleEditor() sees left=false, leftPrev=true,
	// pendingClick=true, !camDragged → tryAddNode() → BUG!

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

// TestMultiTouchCooldownExpires verifies that the multi-touch cooldown
// activates when a two-finger gesture ends, then clears after the
// specified number of frames.
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

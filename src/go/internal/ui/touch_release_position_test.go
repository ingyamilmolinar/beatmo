//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestTouchReleasePosition_HoldsLiftCoords verifies that on the frame a single
// touch ends, updateTouchOverride keeps publishing the *last touch position*
// (with the button reported as released) so that any captured handler whose
// OnRelease fires that frame sees the lift coords instead of a stale (0,0)
// desktop-cursor reading.
//
// Pre-fix: when globalTouchState removes the ended touch and ActiveTouchCount
// drops to 0, updateTouchOverride sets touchOverrideActive=false on the same
// frame the dispatcher reads cursorPosition() for the release branch — so
// OnRelease handlers receive (0,0) and any cursor-position-dependent commit
// (e.g., Knob.updateFromDrag on release) snaps to an extreme.
//
// Post-fix: the override holds the last single-touch position for exactly one
// post-end frame with touchOverrideLeft=false.
func TestTouchReleasePosition_HoldsLiftCoords(t *testing.T) {
	// Use the real globalTouchState so we exercise the same code paths the
	// dispatcher does. Reset before and after to avoid leaking state.
	globalTouchState.Reset()
	resetTouchOverride()
	t.Cleanup(func() {
		globalTouchState.Reset()
		resetTouchOverride()
	})

	mock := newMockTouchState()
	restore := SetTouchForTest(mock.TouchIDs, mock.TouchPosition)
	defer restore()

	// Frame 1: touch down at (300, 400)
	mock.addTouch(1, 300, 400)
	globalTouchState.Update()
	updateTouchOverride()
	cx, cy := cursorPosition()
	if cx != 300 || cy != 400 {
		t.Fatalf("frame 1 (press): cursorPosition=(%d,%d), want (300,400)", cx, cy)
	}
	if !isMouseButtonPressed(ebiten.MouseButtonLeft) {
		t.Fatal("frame 1 (press): isMouseButtonPressed=false, want true")
	}

	// Frame 2: drag up to (300, 370)
	mock.moveTouch(1, 300, 370)
	globalTouchState.Update()
	updateTouchOverride()
	cx, cy = cursorPosition()
	if cx != 300 || cy != 370 {
		t.Fatalf("frame 2 (drag): cursorPosition=(%d,%d), want (300,370)", cx, cy)
	}
	if !isMouseButtonPressed(ebiten.MouseButtonLeft) {
		t.Fatal("frame 2 (drag): isMouseButtonPressed=false, want true")
	}

	// Frame 3: finger lifts. globalTouchState.Update removes the touch;
	// updateTouchOverride MUST publish one final override frame with the lift
	// position and button=false so the dispatcher's release branch sees the
	// correct lift coords.
	mock.removeTouch(1)
	globalTouchState.Update()
	updateTouchOverride()
	cx, cy = cursorPosition()
	if cx != 300 || cy != 370 {
		t.Fatalf("frame 3 (release): cursorPosition=(%d,%d), want (300,370) "+
			"— this is the bug: the override deactivated on the release frame, "+
			"so the dispatcher hands OnRelease handlers stale (0,0) coords", cx, cy)
	}
	if isMouseButtonPressed(ebiten.MouseButtonLeft) {
		t.Fatal("frame 3 (release): isMouseButtonPressed=true, want false")
	}

	// Frame 4: a full frame after release with no touches — override fully
	// clears. cursorPosition falls through to the underlying Ebiten cursor.
	globalTouchState.Update()
	updateTouchOverride()
	if touchOverrideActive {
		t.Fatal("frame 4 (idle): touchOverrideActive=true, want false (override should have cleared)")
	}
	if isMouseButtonPressed(ebiten.MouseButtonLeft) {
		t.Fatal("frame 4 (idle): isMouseButtonPressed=true, want false")
	}
}

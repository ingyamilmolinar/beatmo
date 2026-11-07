//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// On the frame a touch ends the override keeps publishing its last position with the button up, so an
// OnRelease that frame sees the lift point, not a stale (0,0) that snaps a knob to an extreme.
func TestTouchReleasePosition_HoldsLiftCoords(t *testing.T) {
	// Defaults first: a stub helper whose restore never ran would leak its mock into this real-wrapper test.
	resetInputForTest()
	// The real globalTouchState, reset around the test.
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

	// One final override frame with the lift position and the button up.
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

	// Fully cleared: the cursor falls through to Ebiten's.
	globalTouchState.Update()
	updateTouchOverride()
	if touchOverrideActive {
		t.Fatal("frame 4 (idle): touchOverrideActive=true, want false (override should have cleared)")
	}
	if isMouseButtonPressed(ebiten.MouseButtonLeft) {
		t.Fatal("frame 4 (idle): isMouseButtonPressed=true, want false")
	}
}

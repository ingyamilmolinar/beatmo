//go:build test

package ui

import (
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

// mockTouchState holds mock touch data for testing.
type mockTouchState struct {
	ids []ebiten.TouchID
	pos map[ebiten.TouchID]struct{ x, y int }
}

func newMockTouchState() *mockTouchState {
	return &mockTouchState{
		pos: make(map[ebiten.TouchID]struct{ x, y int }),
	}
}

func (m *mockTouchState) addTouch(id ebiten.TouchID, x, y int) {
	m.ids = append(m.ids, id)
	m.pos[id] = struct{ x, y int }{x, y}
}

func (m *mockTouchState) moveTouch(id ebiten.TouchID, x, y int) {
	m.pos[id] = struct{ x, y int }{x, y}
}

func (m *mockTouchState) removeTouch(id ebiten.TouchID) {
	newIDs := make([]ebiten.TouchID, 0, len(m.ids))
	for _, tid := range m.ids {
		if tid != id {
			newIDs = append(newIDs, tid)
		}
	}
	m.ids = newIDs
	delete(m.pos, id)
}

func (m *mockTouchState) TouchIDs() []ebiten.TouchID {
	return m.ids
}

func (m *mockTouchState) TouchPosition(id ebiten.TouchID) (int, int) {
	if p, ok := m.pos[id]; ok {
		return p.x, p.y
	}
	return 0, 0
}

func TestTapDetection(t *testing.T) {
	ts := NewTouchState()
	mock := newMockTouchState()

	// Set up mock touch functions
	restore := SetTouchForTest(mock.TouchIDs, mock.TouchPosition)
	defer restore()

	// Simulate touch down
	mock.addTouch(1, 100, 100)
	gesture := ts.Update()

	// No gesture on touch down
	if gesture != nil {
		t.Errorf("Expected no gesture on touch down, got %v", gesture.Kind)
	}

	// Verify touch is tracked
	if ts.ActiveTouchCount() != 1 {
		t.Errorf("Expected 1 active touch, got %d", ts.ActiveTouchCount())
	}

	// Simulate touch up (quick release for tap)
	mock.removeTouch(1)
	gesture = ts.Update()

	// Should detect tap
	if gesture == nil {
		t.Fatal("Expected tap gesture, got nil")
	}
	if gesture.Kind != GestureTap {
		t.Errorf("Expected GestureTap, got %v", gesture.Kind)
	}
	if gesture.X != 100 || gesture.Y != 100 {
		t.Errorf("Expected tap at (100,100), got (%d,%d)", gesture.X, gesture.Y)
	}
}

func TestTapRejectedWithMovement(t *testing.T) {
	ts := NewTouchState()
	mock := newMockTouchState()

	restore := SetTouchForTest(mock.TouchIDs, mock.TouchPosition)
	defer restore()

	// Touch down
	mock.addTouch(1, 100, 100)
	ts.Update()

	// Move more than tap threshold
	mock.moveTouch(1, 100+tapMaxMovePx+5, 100)
	ts.Update()

	// Touch up
	mock.removeTouch(1)
	gesture := ts.Update()

	// Should NOT detect tap due to movement
	if gesture != nil && gesture.Kind == GestureTap {
		t.Error("Expected no tap gesture due to movement, but got tap")
	}
}

func TestLongPressDetection(t *testing.T) {
	ts := NewTouchState()
	mock := newMockTouchState()

	restore := SetTouchForTest(mock.TouchIDs, mock.TouchPosition)
	defer restore()

	// Touch down
	mock.addTouch(1, 200, 200)
	ts.Update()

	// Manually set start time to simulate long press
	if pt := ts.GetTouch(1); pt != nil {
		pt.StartTime = time.Now().Add(-time.Duration(longPressThresholdMS+100) * time.Millisecond)
	}

	// Update should detect long press
	gesture := ts.Update()

	if gesture == nil {
		t.Fatal("Expected long press gesture, got nil")
	}
	if gesture.Kind != GestureLongPress {
		t.Errorf("Expected GestureLongPress, got %v", gesture.Kind)
	}
	if gesture.X != 200 || gesture.Y != 200 {
		t.Errorf("Expected long press at (200,200), got (%d,%d)", gesture.X, gesture.Y)
	}
}

func TestPinchScaleCalculation(t *testing.T) {
	ts := NewTouchState()
	mock := newMockTouchState()

	restore := SetTouchForTest(mock.TouchIDs, mock.TouchPosition)
	defer restore()

	// Two touches at initial distance of 100
	mock.addTouch(1, 100, 200)
	mock.addTouch(2, 200, 200)
	ts.Update() // Initialize two-finger tracking

	// Move fingers apart (distance 150)
	mock.moveTouch(1, 75, 200)
	mock.moveTouch(2, 225, 200)
	gesture := ts.Update()

	if gesture == nil {
		t.Fatal("Expected pinch gesture, got nil")
	}
	if gesture.Kind != GesturePinch {
		t.Errorf("Expected GesturePinch, got %v", gesture.Kind)
	}

	// Scale should be > 1 (fingers moved apart)
	if gesture.Scale <= 1.0 {
		t.Errorf("Expected scale > 1 for pinch out, got %f", gesture.Scale)
	}

	// Center should be at midpoint
	expectedCX := (75 + 225) / 2
	expectedCY := 200
	if gesture.CenterX != expectedCX || gesture.CenterY != expectedCY {
		t.Errorf("Expected center at (%d,%d), got (%d,%d)", expectedCX, expectedCY, gesture.CenterX, gesture.CenterY)
	}
}

func TestTwoFingerPanDeltaCalculation(t *testing.T) {
	ts := NewTouchState()
	mock := newMockTouchState()

	restore := SetTouchForTest(mock.TouchIDs, mock.TouchPosition)
	defer restore()

	// Two touches at same distance
	mock.addTouch(1, 100, 200)
	mock.addTouch(2, 200, 200)
	ts.Update()

	// Move both fingers in same direction (pan, not pinch)
	// Keep distance the same but move center
	mock.moveTouch(1, 110, 210)
	mock.moveTouch(2, 210, 210)
	gesture := ts.Update()

	if gesture == nil {
		t.Fatal("Expected pan gesture, got nil")
	}

	// Should be either pan or pinch depending on which threshold is hit first
	if gesture.Kind != GestureTwoFingerPan && gesture.Kind != GesturePinch {
		t.Errorf("Expected GestureTwoFingerPan or GesturePinch, got %v", gesture.Kind)
	}

	if gesture.Kind == GestureTwoFingerPan {
		// Verify delta
		if gesture.DeltaX != 10 || gesture.DeltaY != 10 {
			t.Errorf("Expected delta (10,10), got (%d,%d)", gesture.DeltaX, gesture.DeltaY)
		}
	}
}

func TestTouchDistanceCalculation(t *testing.T) {
	tests := []struct {
		x1, y1, x2, y2 int
		expected       float64
	}{
		{0, 0, 3, 4, 5.0},   // 3-4-5 triangle
		{0, 0, 0, 10, 10.0}, // vertical
		{0, 0, 10, 0, 10.0}, // horizontal
		{0, 0, 0, 0, 0.0},   // same point
	}

	for _, tt := range tests {
		result := touchDistance(tt.x1, tt.y1, tt.x2, tt.y2)
		if abs(int(result-tt.expected)) > 0 {
			t.Errorf("touchDistance(%d,%d,%d,%d) = %f, want %f",
				tt.x1, tt.y1, tt.x2, tt.y2, result, tt.expected)
		}
	}
}

func TestTouchStateReset(t *testing.T) {
	ts := NewTouchState()
	mock := newMockTouchState()

	restore := SetTouchForTest(mock.TouchIDs, mock.TouchPosition)
	defer restore()

	// Add touches
	mock.addTouch(1, 100, 100)
	mock.addTouch(2, 200, 200)
	ts.Update()

	if ts.ActiveTouchCount() != 2 {
		t.Errorf("Expected 2 touches, got %d", ts.ActiveTouchCount())
	}

	// Reset
	ts.Reset()

	if ts.ActiveTouchCount() != 0 {
		t.Errorf("Expected 0 touches after reset, got %d", ts.ActiveTouchCount())
	}
	if ts.isPinching || ts.isPanning {
		t.Error("Gesture state should be reset")
	}
}

func TestSingleFingerDrag(t *testing.T) {
	ts := NewTouchState()
	mock := newMockTouchState()

	restore := SetTouchForTest(mock.TouchIDs, mock.TouchPosition)
	defer restore()

	// Touch down
	mock.addTouch(1, 100, 100)
	ts.Update()

	// Manually set start time slightly in the past to allow drag detection
	if pt := ts.GetTouch(1); pt != nil {
		pt.StartTime = time.Now().Add(-100 * time.Millisecond)
	}

	// Move finger
	mock.moveTouch(1, 120, 110)
	gesture := ts.Update()

	if gesture == nil {
		t.Fatal("Expected drag gesture, got nil")
	}
	if gesture.Kind != GestureSingleFingerDrag {
		t.Errorf("Expected GestureSingleFingerDrag, got %v", gesture.Kind)
	}
	if gesture.DeltaX != 20 || gesture.DeltaY != 10 {
		t.Errorf("Expected delta (20,10), got (%d,%d)", gesture.DeltaX, gesture.DeltaY)
	}
}

// TestLongPressOnlyFiresOnce verifies that long-press fires exactly once,
// not on every frame. This was a critical bug where long-press would fire
// 60+ times per second, blocking input and opening menus repeatedly.
func TestLongPressOnlyFiresOnce(t *testing.T) {
	ts := NewTouchState()
	mock := newMockTouchState()

	restore := SetTouchForTest(mock.TouchIDs, mock.TouchPosition)
	defer restore()

	// Touch down
	mock.addTouch(1, 200, 200)
	ts.Update()

	// Set start time to simulate long press threshold exceeded
	if pt := ts.GetTouch(1); pt != nil {
		pt.StartTime = time.Now().Add(-time.Duration(longPressThresholdMS+100) * time.Millisecond)
	}

	// First update should detect long press
	gesture := ts.Update()
	if gesture == nil || gesture.Kind != GestureLongPress {
		t.Fatal("Expected first update to return GestureLongPress")
	}

	// Subsequent updates should NOT return long press again (one-shot behavior)
	longPressCount := 1
	for i := 0; i < 10; i++ {
		gesture = ts.Update()
		if gesture != nil && gesture.Kind == GestureLongPress {
			longPressCount++
		}
	}

	if longPressCount > 1 {
		t.Errorf("Long-press fired %d times, expected exactly 1 (one-shot)", longPressCount)
	}
}

// TestDragAfterLongPress verifies that drag detection works after a long-press
// has fired. Previously, once long-press fired, drag detection was blocked.
func TestDragAfterLongPress(t *testing.T) {
	ts := NewTouchState()
	mock := newMockTouchState()

	restore := SetTouchForTest(mock.TouchIDs, mock.TouchPosition)
	defer restore()

	// Touch down
	mock.addTouch(1, 200, 200)
	ts.Update()

	// Set start time to exceed long press threshold
	if pt := ts.GetTouch(1); pt != nil {
		pt.StartTime = time.Now().Add(-time.Duration(longPressThresholdMS+100) * time.Millisecond)
	}

	// First update should fire long press
	gesture := ts.Update()
	if gesture == nil || gesture.Kind != GestureLongPress {
		t.Fatal("Expected long press to fire first")
	}

	// Now move the finger - should detect drag
	mock.moveTouch(1, 250, 250)
	gesture = ts.Update()

	if gesture == nil {
		t.Fatal("Expected drag gesture after long press, got nil")
	}
	if gesture.Kind != GestureSingleFingerDrag {
		t.Errorf("Expected GestureSingleFingerDrag after long press, got %v", gesture.Kind)
	}
}

// TestSustainedDragAfter500ms verifies that drag continues working after
// 500ms+ of contact. Previously, drags would stop working after ~500ms
// because the long-press condition kept firing and blocking drag detection.
func TestSustainedDragAfter500ms(t *testing.T) {
	ts := NewTouchState()
	mock := newMockTouchState()

	restore := SetTouchForTest(mock.TouchIDs, mock.TouchPosition)
	defer restore()

	// Touch down at position with immediate movement to avoid long-press
	mock.addTouch(1, 100, 100)
	ts.Update()

	// Set start time to exceed long press threshold
	if pt := ts.GetTouch(1); pt != nil {
		pt.StartTime = time.Now().Add(-time.Duration(longPressThresholdMS+200) * time.Millisecond)
	}

	// Move finger beyond the tap threshold to mark this as a drag gesture
	// (this should prevent long-press from firing since we've moved)
	mock.moveTouch(1, 100+tapMaxMovePx+20, 100+tapMaxMovePx+20)
	gesture := ts.Update()

	// Should get drag, not long press (we moved beyond threshold)
	if gesture == nil {
		t.Fatal("Expected drag gesture, got nil")
	}
	if gesture.Kind == GestureLongPress {
		t.Fatal("Got long press when drag was expected (movement should prevent long-press)")
	}
	if gesture.Kind != GestureSingleFingerDrag {
		t.Errorf("Expected GestureSingleFingerDrag, got %v", gesture.Kind)
	}

	// Continue dragging - should keep working
	for i := 0; i < 5; i++ {
		x := 100 + tapMaxMovePx + 20 + (i+1)*10
		y := 100 + tapMaxMovePx + 20 + (i+1)*10
		mock.moveTouch(1, x, y)
		gesture = ts.Update()

		if gesture == nil {
			t.Fatalf("Iteration %d: Expected drag gesture during sustained drag, got nil", i)
		}
		if gesture.Kind != GestureSingleFingerDrag {
			t.Errorf("Iteration %d: Expected GestureSingleFingerDrag, got %v", i, gesture.Kind)
		}
	}
}

// TestLongPressNotFiredOnSlowDragAtTapBoundary is a regression guard for the
// mobile "single-finger drag doesn't pan the camera" bug (touch_gestures.browser.test.js
// Test 1, flaky under parallel-job CPU load). A single-finger drag over/near a node
// on a throttled game loop can have the finger sitting exactly tapMaxMovePx from its
// start at the instant the 500 ms long-press timer elapses. The instantaneous dx/dy
// check alone (dx <= tapMaxMovePx) is satisfied at that boundary, so a spurious
// long-press used to fire, open the quick-action popup, flip panOK false, and freeze
// the pan for the rest of the gesture. The movedBeyondTap latch (set at >= tapMaxMovePx,
// checked in Update BEFORE gesture detection) disqualifies the touch from long-press
// once it has moved. Expect a drag, never a long-press.
func TestLongPressNotFiredOnSlowDragAtTapBoundary(t *testing.T) {
	ts := NewTouchState()
	mock := newMockTouchState()

	restore := SetTouchForTest(mock.TouchIDs, mock.TouchPosition)
	defer restore()

	// Touch down; StartX/StartY = (100,100).
	mock.addTouch(1, 100, 100)
	ts.Update()

	// Simulate a slow drag that has been in contact past the long-press threshold.
	if pt := ts.GetTouch(1); pt != nil {
		pt.StartTime = time.Now().Add(-time.Duration(longPressThresholdMS+100) * time.Millisecond)
	}

	// Finger now sits EXACTLY tapMaxMovePx from the start — the worst-case boundary
	// where the old instantaneous dx <= tapMaxMovePx check still passes.
	mock.moveTouch(1, 100+tapMaxMovePx, 100)
	gesture := ts.Update()

	if gesture != nil && gesture.Kind == GestureLongPress {
		t.Fatalf("slow drag at the tap boundary fired a long-press (would freeze camera pan); "+
			"expected a drag. dx=%d, tapMaxMovePx=%d", tapMaxMovePx, tapMaxMovePx)
	}
	if gesture == nil || gesture.Kind != GestureSingleFingerDrag {
		t.Fatalf("expected GestureSingleFingerDrag, got %v", gesture)
	}

	// The disqualification is sticky: even if a later frame reads the finger back
	// within tapMaxMovePx of the start, no long-press may fire.
	mock.moveTouch(1, 100+tapMaxMovePx-2, 100)
	if g2 := ts.Update(); g2 != nil && g2.Kind == GestureLongPress {
		t.Fatalf("long-press fired after finger returned within tap threshold; movedBeyondTap must latch")
	}
}

// TestLongPressFiredResetOnTouchEnd verifies that the longPressFired flag
// is properly reset when the touch ends, allowing the next touch to fire
// long-press again.
func TestLongPressFiredResetOnTouchEnd(t *testing.T) {
	ts := NewTouchState()
	mock := newMockTouchState()

	restore := SetTouchForTest(mock.TouchIDs, mock.TouchPosition)
	defer restore()

	// First touch with long press
	mock.addTouch(1, 200, 200)
	ts.Update()
	if pt := ts.GetTouch(1); pt != nil {
		pt.StartTime = time.Now().Add(-time.Duration(longPressThresholdMS+100) * time.Millisecond)
	}
	gesture := ts.Update()
	if gesture == nil || gesture.Kind != GestureLongPress {
		t.Fatal("Expected first long press")
	}

	// Release touch
	mock.removeTouch(1)
	ts.Update()

	// Second touch - should be able to fire long press again
	mock.addTouch(2, 300, 300)
	ts.Update()
	if pt := ts.GetTouch(2); pt != nil {
		pt.StartTime = time.Now().Add(-time.Duration(longPressThresholdMS+100) * time.Millisecond)
	}
	gesture = ts.Update()

	if gesture == nil || gesture.Kind != GestureLongPress {
		t.Errorf("Expected second long press after touch end, got %v", gesture)
	}
}

// ─── Pinch-to-zoom direct scale mapping tests ────────────────────────────────

// TestPinchDirectScaleMapping verifies 1:1 ratio: 1.5x finger spread = 1.5x
// camera zoom, 2.0x = 2.0x, 0.5x = 0.5x. Also verifies baseline reset.
func TestPinchDirectScaleMapping(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	initialScale := g.cam.Scale
	cx, cy := 400, 100 // inside grid pane

	// First call records baseline, no zoom yet
	g.handleTouchPinch(cx, cy, 1.0)
	if g.cam.Scale != initialScale {
		t.Fatalf("First pinch call should not change scale, got %f want %f", g.cam.Scale, initialScale)
	}

	// 1.5x finger spread → 1.5x camera zoom
	g.handleTouchPinch(cx, cy, 1.5)
	expected := initialScale * 1.5
	if diff := g.cam.Scale - expected; diff < -0.01 || diff > 0.01 {
		t.Errorf("1.5x spread: got scale %f, want %f", g.cam.Scale, expected)
	}

	// 2.0x finger spread (from same baseline) → 2.0x camera zoom
	g.handleTouchPinch(cx, cy, 2.0)
	expected = initialScale * 2.0
	if diff := g.cam.Scale - expected; diff < -0.01 || diff > 0.01 {
		t.Errorf("2.0x spread: got scale %f, want %f", g.cam.Scale, expected)
	}

	// 0.5x finger spread → 0.5x camera zoom
	g.handleTouchPinch(cx, cy, 0.5)
	expected = initialScale * 0.5
	if diff := g.cam.Scale - expected; diff < -0.01 || diff > 0.01 {
		t.Errorf("0.5x spread: got scale %f, want %f", g.cam.Scale, expected)
	}

	// Reset baseline (simulates gesture end + new gesture)
	g.pinchBaseScale = 0
	g.pinchBaseGestureScale = 0
	scaleAfterReset := g.cam.Scale

	// New gesture: first call records new baseline
	g.handleTouchPinch(cx, cy, 1.0)
	if g.cam.Scale != scaleAfterReset {
		t.Fatalf("New gesture baseline should not change scale")
	}

	// 1.5x from new baseline
	g.handleTouchPinch(cx, cy, 1.5)
	expected = scaleAfterReset * 1.5
	if diff := g.cam.Scale - expected; diff < -0.01 || diff > 0.01 {
		t.Errorf("After reset 1.5x spread: got scale %f, want %f", g.cam.Scale, expected)
	}
}

// TestPinchClampLimits verifies extreme zoom values are clamped to [0.1, 10.0].
func TestPinchClampLimits(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	cx, cy := 400, 100

	// Set camera to scale 5.0, then try 3x pinch → 15.0, should clamp to 10.0
	g.cam.Scale = 5.0
	g.handleTouchPinch(cx, cy, 1.0) // baseline
	g.handleTouchPinch(cx, cy, 3.0) // 5.0 * 3.0 = 15.0 → clamped
	if g.cam.Scale != 10.0 {
		t.Errorf("Expected scale clamped to 10.0, got %f", g.cam.Scale)
	}

	// Reset and test lower clamp
	g.pinchBaseScale = 0
	g.pinchBaseGestureScale = 0
	g.cam.Scale = 0.5
	g.handleTouchPinch(cx, cy, 1.0) // baseline
	g.handleTouchPinch(cx, cy, 0.1) // 0.5 * 0.1 = 0.05 → clamped
	if g.cam.Scale != 0.1 {
		t.Errorf("Expected scale clamped to 0.1, got %f", g.cam.Scale)
	}
}

// TestPinchAnchorPreservation verifies that the world coordinate under the
// pinch center stays fixed after zoom (anchor behavior).
func TestPinchAnchorPreservation(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	cx, cy := 400, 100

	// Compute world coordinate under pinch center before zoom
	sx, sy := float64(cx), float64(cy)
	wxBefore := (sx - g.cam.OffsetX) / g.cam.Scale
	wyBefore := (sy - float64(gridTopOffset()) - g.cam.OffsetY) / g.cam.Scale

	// Pinch zoom 2x
	g.handleTouchPinch(cx, cy, 1.0) // baseline
	g.handleTouchPinch(cx, cy, 2.0) // 2x zoom

	// Compute world coordinate under same screen point after zoom
	wxAfter := (sx - g.cam.OffsetX) / g.cam.Scale
	wyAfter := (sy - float64(gridTopOffset()) - g.cam.OffsetY) / g.cam.Scale

	// World coordinates should be preserved (within rounding tolerance from Snap)
	if diff := wxAfter - wxBefore; diff < -1.5 || diff > 1.5 {
		t.Errorf("World X shifted: before=%f after=%f (diff=%f)", wxBefore, wxAfter, diff)
	}
	if diff := wyAfter - wyBefore; diff < -1.5 || diff > 1.5 {
		t.Errorf("World Y shifted: before=%f after=%f (diff=%f)", wyBefore, wyAfter, diff)
	}
}

// ─── Touch override tests ───────────────────────────────────────────────────

// TestTouchOverrideSingleTouch verifies that a single active touch sets the
// override so cursorPosition() and isMouseButtonPressed() return touch data.
func TestTouchOverrideSingleTouch(t *testing.T) {
	ts := NewTouchState()
	mock := newMockTouchState()
	restore := SetTouchForTest(mock.TouchIDs, mock.TouchPosition)
	defer restore()
	defer resetTouchOverride()
	t.Cleanup(globalTouchState.Reset)

	// Touch down at (300, 250).
	mock.addTouch(1, 300, 250)
	ts.Update()
	// Manually copy into global so updateTouchOverride sees it.
	globalTouchState.points = ts.points

	updateTouchOverride()

	if !touchOverrideActive {
		t.Fatal("Expected touchOverrideActive=true with 1 touch")
	}
	if touchOverrideX != 300 || touchOverrideY != 250 {
		t.Errorf("Expected override pos (300,250), got (%d,%d)", touchOverrideX, touchOverrideY)
	}
	if !touchOverrideLeft {
		t.Error("Expected touchOverrideLeft=true")
	}
}

// TestTouchOverrideMultiTouch verifies that 2+ touches do NOT activate
// the single-touch override (multi-touch gestures are handled separately).
func TestTouchOverrideMultiTouch(t *testing.T) {
	ts := NewTouchState()
	mock := newMockTouchState()
	restore := SetTouchForTest(mock.TouchIDs, mock.TouchPosition)
	defer restore()
	defer resetTouchOverride()
	t.Cleanup(globalTouchState.Reset)

	mock.addTouch(1, 100, 100)
	mock.addTouch(2, 200, 200)
	ts.Update()
	globalTouchState.points = ts.points

	updateTouchOverride()

	if touchOverrideActive {
		t.Error("Expected touchOverrideActive=false with 2 touches")
	}
}

// TestTouchTapInjection verifies the 2-frame tap injection cycle:
// frame 0 → position+left=true, frame 1 → position+left=false, then cleared.
func TestTouchTapInjection(t *testing.T) {
	defer resetTouchOverride()

	injectTouchTap(400, 350)

	// Frame 0: press
	updateTouchOverride()
	if !touchOverrideActive {
		t.Fatal("Frame 0: expected override active")
	}
	if touchOverrideX != 400 || touchOverrideY != 350 {
		t.Errorf("Frame 0: expected (400,350), got (%d,%d)", touchOverrideX, touchOverrideY)
	}
	if !touchOverrideLeft {
		t.Error("Frame 0: expected left=true")
	}

	// Frame 1: release
	updateTouchOverride()
	if !touchOverrideActive {
		t.Fatal("Frame 1: expected override still active")
	}
	if touchOverrideLeft {
		t.Error("Frame 1: expected left=false")
	}

	// Frame 2: cleared
	updateTouchOverride()
	if touchOverrideActive {
		t.Error("Frame 2: expected override inactive")
	}
}

// TestTouchOverrideDisabledDuringTest verifies that inputForTestActive
// suppresses the touch override so test mocks work unimpeded.
func TestTouchOverrideDisabledDuringTest(t *testing.T) {
	ts := NewTouchState()
	mock := newMockTouchState()
	restore := SetTouchForTest(mock.TouchIDs, mock.TouchPosition)
	defer restore()
	defer resetTouchOverride()
	t.Cleanup(globalTouchState.Reset)

	mock.addTouch(1, 300, 250)
	ts.Update()
	globalTouchState.points = ts.points

	// Simulate test input active.
	inputForTestActive = true
	defer func() { inputForTestActive = false }()

	updateTouchOverride()

	if touchOverrideActive {
		t.Error("Expected touchOverrideActive=false when inputForTestActive=true")
	}
}

// TestTouchStateResetClearsLongPressFired verifies that Reset() clears
// the longPressFired flag.
func TestTouchStateResetClearsLongPressFired(t *testing.T) {
	ts := NewTouchState()
	mock := newMockTouchState()

	restore := SetTouchForTest(mock.TouchIDs, mock.TouchPosition)
	defer restore()

	// Trigger long press
	mock.addTouch(1, 200, 200)
	ts.Update()
	if pt := ts.GetTouch(1); pt != nil {
		pt.StartTime = time.Now().Add(-time.Duration(longPressThresholdMS+100) * time.Millisecond)
	}
	ts.Update() // Fire long press

	if !ts.longPressFired {
		t.Fatal("longPressFired should be true after long press")
	}

	// Reset
	ts.Reset()

	if ts.longPressFired {
		t.Error("longPressFired should be false after Reset()")
	}
}

// TestLongPressDeletesNode verifies that long-pressing a node opens the
// quick-action popup (which includes a Delete button) and closes any open menu.
func TestLongPressDeletesNode(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	// Add a node
	n := g.tryAddNode(4, 4, model.NodeTypeRegular)
	if n == nil {
		t.Fatal("Failed to add node")
	}
	nodesBefore := len(g.nodes)

	// Compute screen center of the node
	x1, y1, x2, y2 := g.nodeScreenRect(n)
	cx := int((x1 + x2) / 2)
	cy := int((y1 + y2) / 2)

	// Simulate opening menu for this node (should be cleared by long-press)
	g.sidebar.Open(n)

	g.handleTouchLongPress(cx, cy)

	// Node should NOT be deleted — popup opens instead.
	if len(g.nodes) != nodesBefore {
		t.Errorf("Expected %d nodes (popup, not delete), got %d", nodesBefore, len(g.nodes))
	}
	if !g.longPressPopup {
		t.Error("Long-press popup should be open")
	}
	if g.sidebar.IsOpen() {
		t.Error("Node menu should be closed after long-press popup")
	}
	if g.sidebar.Node() != nil {
		t.Error("nodeMenuNode should be nil after long-press popup")
	}
}

// TestLongPressEmptySpaceNoOp verifies that long-pressing on empty grid
// space does nothing (no node created, no crash).
func TestLongPressEmptySpaceNoOp(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	nodesBefore := len(g.nodes)

	// Long-press on empty space (center of grid, no node there)
	g.handleTouchLongPress(400, 100)

	if len(g.nodes) != nodesBefore {
		t.Errorf("Expected %d nodes (unchanged) after long-press on empty space, got %d", nodesBefore, len(g.nodes))
	}
}

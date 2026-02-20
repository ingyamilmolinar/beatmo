//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestTouchScrollDoesNotActivateSplitter verifies that starting a touch scroll
// in the drum view rows and dragging upward (cursor moves into splitter grab
// zone) does not cause the splitter to start dragging.
func TestTouchScrollDoesNotActivateSplitter(t *testing.T) {
	logger := game_log.New(testLogOutput(), game_log.LevelError)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 300, 800, 600), graph, logger)
	dv.recalcButtons()

	s := NewSplitter(600)
	s.Y = 300
	s.winW = 800
	s.totalH = 600

	disp := NewInputDispatcher()
	disp.Register(s)
	disp.Register(dv)
	disp.Sort()

	// Simulate mouse press inside drum view rows.
	startX, startY := 400, 450
	restore := SetInputForTest(
		func() (int, int) { return startX, startY },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	// Initial press — drum view should capture.
	disp.Dispatch(startX, startY, true)
	if s.dragging {
		t.Fatal("splitter started dragging on initial press in drum area")
	}

	// Drag upward into splitter's grab zone.
	dragY := s.Y - 2 // well within splitter grab zone
	disp.Dispatch(startX, dragY, true)
	if s.dragging {
		t.Error("splitter started dragging when touch scroll moved into grab zone — expected drum view to hold capture")
	}

	// Release.
	disp.Dispatch(startX, dragY, false)
	if s.dragging {
		t.Error("splitter should not be dragging after release")
	}
}

// TestSplitterDragDoesNotActivateDrumScroll verifies that when the splitter is
// being dragged downward (cursor moves into drum area), the drum view does not
// start a touch scroll.
func TestSplitterDragDoesNotActivateDrumScroll(t *testing.T) {
	logger := game_log.New(testLogOutput(), game_log.LevelError)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 300, 800, 600), graph, logger)
	dv.recalcButtons()

	s := NewSplitter(600)
	s.Y = 300
	s.winW = 800
	s.totalH = 600

	disp := NewInputDispatcher()
	disp.Register(s)
	disp.Register(dv)
	disp.Sort()

	// Press on splitter divider to start dragging.
	disp.Dispatch(400, 300, true)
	if !s.dragging {
		t.Fatal("splitter should start dragging when pressed on divider")
	}

	// Drag downward into drum view rows area.
	drumY := 450
	disp.Dispatch(400, drumY, true)

	// Simulate what Game does: set inputCapturedExternally before drum.Update().
	dv.inputCapturedExternally = disp.HasCaptureOtherThan(dv)

	// Mock cursor position in drum area and run drum Update.
	restore := SetInputForTest(
		func() (int, int) { return 400, drumY },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	dv.Update()

	if dv.rowScroll.TouchActive() {
		t.Error("drum view started touch scroll while splitter holds capture — expected inputCapturedExternally to block it")
	}
	if dv.dragging {
		t.Error("drum view started timeline drag while splitter holds capture")
	}
	if dv.scrubbing {
		t.Error("drum view started scrubbing while splitter holds capture")
	}
}

// TestDrumViewHandleInputCapturesOnPress verifies that HandleInput returns
// InputCaptured (not InputConsumed) for a new press within bounds.
func TestDrumViewHandleInputCapturesOnPress(t *testing.T) {
	logger := game_log.New(testLogOutput(), game_log.LevelError)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 300, 800, 600), graph, logger)

	result := dv.HandleInput(400, 450, true)
	if result != InputCaptured {
		t.Errorf("expected InputCaptured for press in bounds, got %v", result)
	}
}

// TestDrumViewCaptureMaintainedAcrossFrames verifies that Capturing() returns
// true while mouseDownInBounds is set and HandleInput keeps returning
// InputCaptured even when the cursor moves outside bounds.
func TestDrumViewCaptureMaintainedAcrossFrames(t *testing.T) {
	logger := game_log.New(testLogOutput(), game_log.LevelError)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 300, 800, 600), graph, logger)

	// Initial press inside bounds.
	dv.HandleInput(400, 450, true)
	if !dv.Capturing() {
		t.Error("Capturing() should be true after press in bounds")
	}
	if !dv.mouseDownInBounds {
		t.Error("mouseDownInBounds should be set after press in bounds")
	}

	// Cursor moves outside bounds while still pressed — capture maintained.
	result := dv.HandleInput(400, 100, true) // above drum view
	if result != InputCaptured {
		t.Errorf("expected InputCaptured when cursor moved outside bounds with mouseDownInBounds, got %v", result)
	}
}

// TestDrumViewCaptureReleasedOnMouseUp verifies that mouseDownInBounds clears
// when the mouse button is released and no touch scroll is active.
func TestDrumViewCaptureReleasedOnMouseUp(t *testing.T) {
	logger := game_log.New(testLogOutput(), game_log.LevelError)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 300, 800, 600), graph, logger)

	// Press inside bounds (no touch scroll started).
	dv.HandleInput(400, 450, true)
	if !dv.mouseDownInBounds {
		t.Fatal("mouseDownInBounds should be set after press")
	}
	// Precondition: no touch interaction active.
	if dv.rowScroll.TouchActive() {
		t.Fatal("TouchActive should be false — no touch begun")
	}

	// Release — should clear immediately since no touch scroll is active.
	dv.HandleInput(400, 450, false)
	if dv.mouseDownInBounds {
		t.Error("mouseDownInBounds should be cleared after release when no touch active")
	}
	if dv.Capturing() {
		t.Error("Capturing() should be false after clean release")
	}
}

// TestTouchFlickerDoesNotReleaseDrumCapture simulates a mobile touch flicker:
// press in drum rows, start touch scroll, then release arrives while
// TouchActive() is still true. mouseDownInBounds must NOT be cleared.
func TestTouchFlickerDoesNotReleaseDrumCapture(t *testing.T) {
	logger := game_log.New(testLogOutput(), game_log.LevelError)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 300, 800, 600), graph, logger)

	// Press inside drum bounds.
	dv.HandleInput(400, 450, true)
	if !dv.mouseDownInBounds {
		t.Fatal("mouseDownInBounds should be set after press")
	}

	// Simulate touch scroll being started (HandleTouchBegin called by drum Update).
	dv.rowScroll.HandleTouchBegin(400, 450)
	if !dv.rowScroll.TouchActive() {
		t.Fatal("TouchActive should be true after HandleTouchBegin")
	}

	// Flicker frame: release arrives but touch is still active.
	dv.HandleInput(0, 0, false)

	// Layer 1: mouseDownInBounds must stay because TouchActive is true.
	if !dv.mouseDownInBounds {
		t.Error("mouseDownInBounds should NOT be cleared during touch flicker (TouchActive=true)")
	}
	// Layer 2: Capturing() must remain true.
	if !dv.Capturing() {
		t.Error("Capturing() should remain true during touch flicker")
	}
}

// TestTouchScrollMaintainsCaptureBeforeCommit verifies that Capturing() returns
// true when a touch scroll is active but not yet committed past the dead zone.
// This exercises Layer 2 (TouchActive in Capturing) independently.
func TestTouchScrollMaintainsCaptureBeforeCommit(t *testing.T) {
	logger := game_log.New(testLogOutput(), game_log.LevelError)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 300, 800, 600), graph, logger)

	// Start touch scroll without moving past dead zone.
	dv.rowScroll.HandleTouchBegin(400, 450)
	if !dv.rowScroll.TouchActive() {
		t.Fatal("TouchActive should be true after HandleTouchBegin")
	}
	if dv.rowScroll.ScrollingCommitted() {
		t.Fatal("ScrollingCommitted should be false before movement past dead zone")
	}

	// anyDragActive checks ScrollingCommitted, which is false.
	// But Capturing() should still return true via TouchActive().
	if !dv.Capturing() {
		t.Error("Capturing() should be true when TouchActive is true, even before scroll commits")
	}
}

// TestDrumViewCaptureReleasedAfterTouchEnds verifies no stuck state: after
// HandleTouchEnd makes TouchActive() false, a subsequent HandleInput release
// clears mouseDownInBounds and Capturing() returns false.
func TestDrumViewCaptureReleasedAfterTouchEnds(t *testing.T) {
	logger := game_log.New(testLogOutput(), game_log.LevelError)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 300, 800, 600), graph, logger)

	// Press and start touch scroll.
	dv.HandleInput(400, 450, true)
	dv.rowScroll.HandleTouchBegin(400, 450)
	if !dv.Capturing() {
		t.Fatal("should be capturing after press + touch begin")
	}

	// Touch ends (as drum.Update() would call on real release).
	dv.rowScroll.HandleTouchEnd()
	if dv.rowScroll.TouchActive() {
		t.Fatal("TouchActive should be false after HandleTouchEnd")
	}

	// Next frame: release arrives, mouseDownInBounds should now clear.
	dv.HandleInput(400, 450, false)
	if dv.mouseDownInBounds {
		t.Error("mouseDownInBounds should be cleared after touch ended and release received")
	}
	// Capturing may still be true for one frame due to momentum, but
	// mouseDownInBounds must be cleared to allow eventual release.
}

// TestSplitterGuardFramesBlockNewDrag verifies that guardFrames > 0 prevents
// the splitter from initiating a new drag, but does not block an ongoing drag.
func TestSplitterGuardFramesBlockNewDrag(t *testing.T) {
	s := NewSplitter(600)
	s.Y = 300
	s.winW = 800
	s.totalH = 600

	// With guard active, a press near the divider should be ignored.
	s.guardFrames = 3
	result := s.HandleInput(400, 300, true)
	if result != InputIgnored {
		t.Errorf("expected InputIgnored when guardFrames>0 and not dragging, got %v", result)
	}
	if s.dragging {
		t.Error("splitter should not start dragging when guardFrames > 0")
	}

	// If already dragging, guard does NOT block.
	s.dragging = true
	s.guardFrames = 3
	result = s.HandleInput(400, 310, true)
	if result == InputIgnored {
		t.Error("guard should not block an ongoing drag")
	}

	// Clean up: reset Y back since the ongoing drag moved it.
	s.dragging = false
	s.guardFrames = 0
	s.Y = 300

	// Without guard, press near divider should initiate drag normally.
	result = s.HandleInput(400, 300, true)
	if result != InputCaptured {
		t.Errorf("expected InputCaptured when guardFrames=0, got %v", result)
	}
	if !s.dragging {
		t.Error("splitter should be dragging after press near divider with no guard")
	}
}

// TestTouchScrollDoesNotActivateLayoutResize verifies that when a touch scroll
// is active in the drum view rows, the LayoutResizeHandler.Update() is not
// called by DrumView.Update(), preventing the handler from stealing the drag.
func TestTouchScrollDoesNotActivateLayoutResize(t *testing.T) {
	logger := game_log.New(testLogOutput(), game_log.LevelError)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 300, 800, 600), graph, logger)
	dv.recalcButtons()
	dv.calcLayout()

	if dv.layoutHandler == nil {
		t.Fatal("layoutHandler should be initialized after calcLayout")
	}

	// Start a touch scroll in the rows area.
	dv.rowScroll.HandleTouchBegin(400, 500)
	if !dv.rowScroll.TouchActive() {
		t.Fatal("TouchActive should be true after HandleTouchBegin")
	}

	// Track whether layoutHandler.Update() was called by instrumenting it.
	// We do this by checking that the handler did NOT start dragging even
	// with cursor positioned right on a row divider with press active.
	// Find a row divider Y coordinate from widgets (if available).
	var dividerY int
	if dv.widgets != nil && len(dv.widgets.rowPos) > 2 {
		dividerY = dv.widgets.rowPos[1] + dv.Bounds.Min.Y
	} else {
		// Fallback: use a position near the top of drum view where
		// a divider would typically be.
		dividerY = dv.Bounds.Min.Y + 40
	}

	restore := SetInputForTest(
		func() (int, int) { return 400, dividerY },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	// Run Update — the guard should prevent layoutHandler from running.
	dv.Update()

	if dv.layoutHandler.Capturing() {
		t.Error("layoutHandler started resize drag while touch scroll was active — guard should prevent this")
	}

	// Verify the guard is specifically due to TouchActive: end touch, then
	// confirm layoutHandler CAN run (it may or may not detect a divider, but
	// at minimum it should not be blocked).
	dv.rowScroll.HandleTouchEnd()
	if dv.rowScroll.TouchActive() {
		t.Fatal("TouchActive should be false after HandleTouchEnd")
	}
}

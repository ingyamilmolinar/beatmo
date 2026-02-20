//go:build test

package ui

import (
	"image"
	"testing"
)

// ─── GridRect / DrumRect geometry accessors ──────────────────────────────────

func TestSplitterGridRectHorizontal(t *testing.T) {
	s := NewSplitter(600)
	s.Y = 250
	s.winW = 800
	s.totalH = 600

	r := s.GridRect(800, 600)
	want := image.Rect(0, 0, 800, 250)
	if r != want {
		t.Errorf("GridRect horizontal: got %v, want %v", r, want)
	}
}

func TestSplitterGridRectVertical(t *testing.T) {
	s := NewSplitter(600)
	s.horizontal = false
	s.X = 350
	s.winW = 800
	s.totalH = 600

	r := s.GridRect(800, 600)
	want := image.Rect(0, 0, 350, 600)
	if r != want {
		t.Errorf("GridRect vertical: got %v, want %v", r, want)
	}
}

func TestSplitterDrumRectHorizontal(t *testing.T) {
	s := NewSplitter(600)
	s.Y = 250
	s.winW = 800
	s.totalH = 600

	r := s.DrumRect(800, 600)
	want := image.Rect(0, 250, 800, 600)
	if r != want {
		t.Errorf("DrumRect horizontal: got %v, want %v", r, want)
	}
}

func TestSplitterDrumRectVertical(t *testing.T) {
	s := NewSplitter(600)
	s.horizontal = false
	s.X = 350
	s.winW = 800
	s.totalH = 600

	r := s.DrumRect(800, 600)
	want := image.Rect(350, 0, 800, 600)
	if r != want {
		t.Errorf("DrumRect vertical: got %v, want %v", r, want)
	}
}

// ─── InGridPane ──────────────────────────────────────────────────────────────

func TestSplitterInGridPane(t *testing.T) {
	s := NewSplitter(600)
	s.Y = 300
	s.winW = 800
	s.totalH = 600

	// Horizontal mode: y < Y is in grid pane
	if !s.InGridPane(400, 200) {
		t.Error("horizontal: point above Y should be in grid pane")
	}
	if s.InGridPane(400, 400) {
		t.Error("horizontal: point below Y should NOT be in grid pane")
	}
	if s.InGridPane(400, 300) {
		t.Error("horizontal: point at Y should NOT be in grid pane (y < Y, not y <= Y)")
	}

	// Vertical mode: x < X is in grid pane
	s.horizontal = false
	s.X = 400
	if !s.InGridPane(200, 300) {
		t.Error("vertical: point left of X should be in grid pane")
	}
	if s.InGridPane(500, 300) {
		t.Error("vertical: point right of X should NOT be in grid pane")
	}
	if s.InGridPane(400, 300) {
		t.Error("vertical: point at X should NOT be in grid pane (x < X, not x <= X)")
	}
}

// ─── GridW / GridH ───────────────────────────────────────────────────────────

func TestSplitterGridWH(t *testing.T) {
	s := NewSplitter(600)
	s.Y = 250
	s.X = 350
	s.winW = 800
	s.totalH = 600

	// Horizontal mode
	s.horizontal = true
	if gw := s.GridW(800); gw != 800 {
		t.Errorf("horizontal GridW: got %d, want 800", gw)
	}
	if gh := s.GridH(600); gh != 250 {
		t.Errorf("horizontal GridH: got %d, want 250", gh)
	}

	// Vertical mode
	s.horizontal = false
	if gw := s.GridW(800); gw != 350 {
		t.Errorf("vertical GridW: got %d, want 350", gw)
	}
	if gh := s.GridH(600); gh != 600 {
		t.Errorf("vertical GridH: got %d, want 600", gh)
	}
}

// ─── UpdateResize side-by-side (vertical) mode ──────────────────────────────

func TestSplitterUpdateResizeSideBySide(t *testing.T) {
	s := NewSplitter(600)
	s.horizontal = false
	s.X = 400
	s.ratio = 0.5
	s.userSet = true

	s.UpdateResize(600, 800)

	// X should be ratio * winW = 0.5 * 800 = 400
	if s.X != 400 {
		t.Errorf("side-by-side X: got %d, want 400", s.X)
	}

	// X should be clamped to [120, winW-120]
	s.X = 50
	s.ratio = float64(50) / float64(800)
	s.UpdateResize(600, 800)
	if s.X < 120 {
		t.Errorf("side-by-side X not clamped to min 120: got %d", s.X)
	}

	s.X = 750
	s.ratio = float64(750) / float64(800)
	s.UpdateResize(600, 800)
	maxX := 800 - 120
	if s.X > maxX {
		t.Errorf("side-by-side X not clamped to max %d: got %d", maxX, s.X)
	}

	// Y should be set to totalH in side-by-side mode
	if s.Y != 600 {
		t.Errorf("side-by-side Y should equal totalH: got %d, want 600", s.Y)
	}
}

func TestSplitterUpdateResizeSideBySideMobile(t *testing.T) {
	forceSmallScreenForTest = true
	defer func() { forceSmallScreenForTest = false }()

	winW := 800
	s := NewSplitter(600)
	s.horizontal = false
	s.userSet = true

	// Set X very low; on mobile, quarter bound (800/4=200) > 120 so minX = 200
	s.X = 100
	s.ratio = float64(100) / float64(winW)
	s.UpdateResize(600, winW)

	minX := winW / 4 // 200
	if s.X < minX {
		t.Errorf("mobile side-by-side X should be clamped to quarter bound %d: got %d", minX, s.X)
	}

	// Set X very high; maxX = winW - winW/4 = 600
	s.X = 750
	s.ratio = float64(750) / float64(winW)
	s.UpdateResize(600, winW)

	maxX := winW - winW/4 // 600
	if s.X > maxX {
		t.Errorf("mobile side-by-side X should be clamped to quarter bound %d: got %d", maxX, s.X)
	}
}

func TestSplitterUpdateResizeKeepsYInSideBySide(t *testing.T) {
	s := NewSplitter(600)
	s.horizontal = false
	s.X = 400

	s.UpdateResize(700, 800)
	if s.Y != 700 {
		t.Errorf("side-by-side Y should track totalH: got %d, want 700", s.Y)
	}

	s.UpdateResize(500, 800)
	if s.Y != 500 {
		t.Errorf("side-by-side Y should track totalH: got %d, want 500", s.Y)
	}
}

// ─── HandleInput side-by-side drag ──────────────────────────────────────────

func TestSplitterHandleInputVerticalDrag(t *testing.T) {
	s := NewSplitter(600)
	s.horizontal = false
	s.X = 400
	s.winW = 800
	s.totalH = 600

	// Click on the handle center (X, totalH/2) to start drag
	result := s.HandleInput(400, 300, true)
	if result != InputCaptured {
		t.Errorf("expected InputCaptured on handle click, got %v", result)
	}
	if !s.dragging {
		t.Fatal("expected drag to start")
	}

	// Drag to X=500 (second frame, wasDragging is true so position updates)
	result = s.HandleInput(500, 300, true)
	if result != InputCaptured {
		t.Errorf("expected InputCaptured during drag, got %v", result)
	}
	if s.X != 500 {
		t.Errorf("X should follow drag: got %d, want 500", s.X)
	}
	expectedRatio := float64(500) / float64(800)
	if s.ratio != expectedRatio {
		t.Errorf("ratio should update: got %f, want %f", s.ratio, expectedRatio)
	}

	// Release
	result = s.HandleInput(500, 300, false)
	if result != InputConsumed {
		t.Errorf("expected InputConsumed on release, got %v", result)
	}
	if s.dragging {
		t.Error("drag should end on release")
	}
}

func TestSplitterHandleInputVerticalClamping(t *testing.T) {
	s := NewSplitter(600)
	s.horizontal = false
	s.X = 400
	s.winW = 800
	s.totalH = 600

	// Start drag
	s.HandleInput(400, 300, true)
	if !s.dragging {
		t.Fatal("expected drag to start")
	}

	// Drag to X=50, should be clamped to 120
	s.HandleInput(50, 300, true)
	if s.X < 120 {
		t.Errorf("X should be clamped to min 120: got %d", s.X)
	}

	// Drag to X=750, should be clamped to 680
	s.HandleInput(750, 300, true)
	maxX := 800 - 120
	if s.X > maxX {
		t.Errorf("X should be clamped to max %d: got %d", maxX, s.X)
	}

	s.HandleInput(500, 300, false) // release
}

func TestSplitterHandleInputVerticalClampingMobile(t *testing.T) {
	forceSmallScreenForTest = true
	defer func() { forceSmallScreenForTest = false }()

	s := NewSplitter(600)
	s.horizontal = false
	s.X = 400
	s.winW = 800
	s.totalH = 600

	// Start drag
	s.HandleInput(400, 300, true)
	if !s.dragging {
		t.Fatal("expected drag to start")
	}

	// Drag to X=100; mobile quarter bound = 800/4=200 > 120 so minX = 200
	s.HandleInput(100, 300, true)
	minX := s.winW / 4
	if s.X < minX {
		t.Errorf("mobile: X should be clamped to quarter bound %d: got %d", minX, s.X)
	}

	// Drag to X=750; mobile maxX = 800 - 800/4 = 600
	s.HandleInput(750, 300, true)
	maxX := s.winW - s.winW/4
	if s.X > maxX {
		t.Errorf("mobile: X should be clamped to quarter bound %d: got %d", maxX, s.X)
	}

	s.HandleInput(500, 300, false) // release
}

// ─── HandleInput guard frames ───────────────────────────────────────────────

func TestSplitterGuardFramesBlockDragInit(t *testing.T) {
	s := NewSplitter(600)
	s.Y = 300
	s.winW = 800
	s.totalH = 600
	s.guardFrames = 3 // cooldown active

	// Click on the handle center - should be blocked by guard frames
	result := s.HandleInput(400, 300, true)
	if result != InputIgnored {
		t.Errorf("guardFrames > 0: expected InputIgnored, got %v", result)
	}
	if s.dragging {
		t.Error("guardFrames > 0: drag should NOT start")
	}
}

func TestSplitterGuardFramesAllowActiveDrag(t *testing.T) {
	s := NewSplitter(600)
	s.Y = 300
	s.winW = 800
	s.totalH = 600

	// Start a drag first (no guard frames)
	s.HandleInput(400, 300, true)
	if !s.dragging {
		t.Fatal("expected drag to start")
	}

	// Now set guardFrames while dragging - should NOT block the active drag
	s.guardFrames = 5
	result := s.HandleInput(400, 350, true)
	if result != InputCaptured {
		t.Errorf("active drag with guardFrames: expected InputCaptured, got %v", result)
	}
	if !s.dragging {
		t.Error("active drag should continue despite guardFrames")
	}

	s.HandleInput(400, 350, false) // release
}

// ─── suppressClicksUntilRelease ─────────────────────────────────────────────

func TestSplitterSuppressClicksUntilRelease(t *testing.T) {
	s := NewSplitter(600)
	s.Y = 300
	s.winW = 800
	s.totalH = 600

	prev := suppressClicksUntilRelease
	defer func() { suppressClicksUntilRelease = prev }()

	suppressClicksUntilRelease = true

	// While suppressed and pressed, HandleInput returns InputIgnored
	result := s.HandleInput(400, 300, true)
	if result != InputIgnored {
		t.Errorf("suppress active + pressed: expected InputIgnored, got %v", result)
	}
	if s.dragging {
		t.Error("suppress active: drag should NOT start")
	}
	if !suppressClicksUntilRelease {
		t.Error("suppress should still be active while pressed")
	}

	// Release clears suppressClicksUntilRelease
	result = s.HandleInput(400, 300, false)
	if result != InputIgnored {
		t.Errorf("suppress active + released: expected InputIgnored, got %v", result)
	}
	if suppressClicksUntilRelease {
		t.Error("suppress should be cleared after release")
	}
}

// ─── InputBounds ─────────────────────────────────────────────────────────────

func TestSplitterInputBoundsHorizontal(t *testing.T) {
	s := NewSplitter(600)
	s.Y = 300
	s.winW = 800
	s.totalH = 600

	grab := TouchGrabZone()
	r := s.InputBounds()

	want := image.Rect(0, 300-grab, 800, 300+4)
	if r != want {
		t.Errorf("horizontal InputBounds: got %v, want %v", r, want)
	}
}

func TestSplitterInputBoundsVertical(t *testing.T) {
	s := NewSplitter(600)
	s.horizontal = false
	s.X = 400
	s.winW = 800
	s.totalH = 600

	grab := TouchGrabZone()
	r := s.InputBounds()

	want := image.Rect(400-grab, 0, 400+4, 600)
	if r != want {
		t.Errorf("vertical InputBounds: got %v, want %v", r, want)
	}
}

// ─── HandleWheel ─────────────────────────────────────────────────────────────

func TestSplitterHandleWheel(t *testing.T) {
	s := NewSplitter(600)
	s.Y = 300
	s.winW = 800
	s.totalH = 600

	result := s.HandleWheel(400, 300, 3)
	if result != InputIgnored {
		t.Errorf("HandleWheel: expected InputIgnored, got %v", result)
	}

	// Also test with different coordinates and negative steps
	result = s.HandleWheel(0, 0, -5)
	if result != InputIgnored {
		t.Errorf("HandleWheel negative steps: expected InputIgnored, got %v", result)
	}
}

// ─── Capturing ───────────────────────────────────────────────────────────────

func TestSplitterCapturing(t *testing.T) {
	s := NewSplitter(600)
	s.Y = 300
	s.winW = 800
	s.totalH = 600

	// Initially not capturing
	if s.Capturing() {
		t.Error("should not be capturing initially")
	}

	// Start drag
	s.HandleInput(400, 300, true)
	if !s.Capturing() {
		t.Error("should be capturing during drag")
	}

	// Release
	s.HandleInput(400, 300, false)
	if s.Capturing() {
		t.Error("should not be capturing after release")
	}
}

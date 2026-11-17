//go:build test

package ui

import (
	"image"
	"testing"
)

// TestSplitter_GuardFrameBlocksDrag verifies that when guardFrames > 0,
// HandleInput returns InputIgnored for new press attempts. After counting
// down guardFrames to 0, a press on the handle initiates drag and returns
// InputCaptured.
func TestSplitter_GuardFrameBlocksDrag(t *testing.T) {
	s := NewSplitter(600)
	s.winW = 800
	s.totalH = 600
	s.UpdateResize(600, 800)

	// Get the handle rect to know where to press.
	handleR := s.HandleRect()
	if handleR.Empty() {
		t.Fatal("handle rect should not be empty")
	}
	hx := (handleR.Min.X + handleR.Max.X) / 2
	hy := (handleR.Min.Y + handleR.Max.Y) / 2

	// Set guard frames to 3.
	s.guardFrames = 3

	// Press on the handle — should be ignored due to guard.
	result := s.HandleInput(hx, hy, true)
	if result != InputIgnored {
		t.Fatalf("expected InputIgnored while guardFrames=%d, got %v", s.guardFrames, result)
	}
	if s.dragging {
		t.Fatal("should not be dragging while guardFrames > 0")
	}

	// Decrement guard 3 times to reach 0.
	s.guardFrames--
	result = s.HandleInput(hx, hy, true)
	if result != InputIgnored {
		t.Fatalf("expected InputIgnored while guardFrames=%d, got %v", s.guardFrames, result)
	}

	s.guardFrames--
	result = s.HandleInput(hx, hy, true)
	if result != InputIgnored {
		t.Fatalf("expected InputIgnored while guardFrames=%d, got %v", s.guardFrames, result)
	}

	s.guardFrames--
	if s.guardFrames != 0 {
		t.Fatalf("guardFrames should be 0, got %d", s.guardFrames)
	}

	// Release first to reset wasPressed state from previous calls.
	s.HandleInput(hx, hy, false)

	// Now press again — guard is 0, should start drag.
	result = s.HandleInput(hx, hy, true)
	if result != InputCaptured {
		t.Fatalf("expected InputCaptured with guardFrames=0, got %v", result)
	}
	if !s.dragging {
		t.Fatal("should be dragging after press with guardFrames=0")
	}

	// Clean up: release.
	s.HandleInput(hx, hy, false)
}

// TestSplitter_SideBySideModeDrag verifies that in side-by-side mode
// (horizontal=false), pressing on the vertical handle initiates drag,
// and dragging changes s.X, clamped to [120, winW-120].
func TestSplitter_SideBySideModeDrag(t *testing.T) {
	winW := 800
	totalH := 600

	s := &Splitter{
		X:          winW / 2,
		ratio:      0.5,
		horizontal: false,
		winW:       winW,
		totalH:     totalH,
	}
	s.UpdateResize(totalH, winW)

	// Verify handle rect is non-empty for vertical mode.
	handleR := s.HandleRect()
	if handleR.Empty() {
		t.Fatal("side-by-side handle rect should not be empty")
	}

	hx := (handleR.Min.X + handleR.Max.X) / 2
	hy := (handleR.Min.Y + handleR.Max.Y) / 2

	// Press on the handle.
	result := s.HandleInput(hx, hy, true)
	if result != InputCaptured {
		t.Fatalf("expected InputCaptured on handle press, got %v", result)
	}
	if !s.dragging {
		t.Fatal("should be dragging after press on handle")
	}

	// Drag to the right by 100px.
	initialX := s.X
	dragX := hx + 100
	result = s.HandleInput(dragX, hy, true)
	if result != InputCaptured {
		t.Fatalf("expected InputCaptured during drag, got %v", result)
	}
	if s.X == initialX {
		t.Errorf("X should have changed after drag: still %d", s.X)
	}

	// Verify clamping at max boundary (winW - 120).
	s.HandleInput(winW+100, hy, true)
	maxX := winW - 120
	if s.X > maxX {
		t.Errorf("X should be clamped to max %d, got %d", maxX, s.X)
	}

	// Release and drag to minimum boundary.
	s.HandleInput(winW+100, hy, false)

	// Re-press on current handle position.
	handleR = s.HandleRect()
	hx = (handleR.Min.X + handleR.Max.X) / 2
	hy = (handleR.Min.Y + handleR.Max.Y) / 2
	s.HandleInput(hx, hy, true)

	// Drag to the far left, past minimum.
	s.HandleInput(0, hy, true)
	minX := 120
	if s.X < minX {
		t.Errorf("X should be clamped to min %d, got %d", minX, s.X)
	}

	// Release.
	s.HandleInput(0, hy, false)
	if s.dragging {
		t.Fatal("should not be dragging after release")
	}

	// Verify it is in side-by-side mode.
	if s.Horizontal() {
		t.Fatal("Horizontal() should return false in side-by-side mode")
	}

	// Grid rect should be left of X.
	gridR := s.GridRect(winW, totalH)
	if gridR.Max.X != s.X {
		t.Errorf("GridRect max X should be s.X=%d, got %d", s.X, gridR.Max.X)
	}

	// Drum rect should be right of X.
	drumR := s.DrumRect(winW, totalH)
	if drumR.Min.X != s.X {
		t.Errorf("DrumRect min X should be s.X=%d, got %d", s.X, drumR.Min.X)
	}

	_ = image.Rect(0, 0, 0, 0) // ensure image is used
}

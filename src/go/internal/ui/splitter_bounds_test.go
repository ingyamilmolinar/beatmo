//go:build test

package ui

import (
	"image"
	"testing"
)

func TestSplitter_RejectsClicksFromBelow(t *testing.T) {
	s := NewSplitter(600)
	s.Y = 300
	s.winW = 800

	// Simulate click from below the splitter (in drum pane area)
	y := s.Y + 100 // Well below the splitter
	result := s.HandleInput(400, y, true)

	if result != InputIgnored {
		t.Errorf("expected InputIgnored for click below splitter, got %v", result)
	}
	if s.dragging {
		t.Error("splitter should not be dragging when clicked from below")
	}
}

func TestSplitter_AcceptsClicksFromAbove(t *testing.T) {
	s := NewSplitter(600)
	s.Y = 300
	s.winW = 800

	// Simulate click from within the grab zone
	y := s.Y // On the divider line
	result := s.HandleInput(400, y, true)

	if result != InputCaptured {
		t.Errorf("expected InputCaptured for click on splitter, got %v", result)
	}
	if !s.dragging {
		t.Error("splitter should be dragging when clicked on grab zone")
	}
}

func TestSplitter_AcceptsClicksFromGridPane(t *testing.T) {
	s := NewSplitter(600)
	s.Y = 300
	s.winW = 800

	// Simulate click from just above the grab zone
	y := s.Y - 3 // Within grab range (5px)
	result := s.HandleInput(400, y, true)

	if result != InputCaptured {
		t.Errorf("expected InputCaptured for click near splitter from above, got %v", result)
	}
	if !s.dragging {
		t.Error("splitter should be dragging when clicked from within grab zone")
	}
}

func TestSplitter_DragContinuesWhenCursorMovesBelow(t *testing.T) {
	s := NewSplitter(600)
	s.Y = 300
	s.winW = 800

	// Start drag from within grab zone
	result := s.HandleInput(400, s.Y, true)
	if result != InputCaptured {
		t.Fatalf("expected InputCaptured to start drag, got %v", result)
	}

	// Continue drag even when cursor moves below
	y := s.Y + 100 // Well below the splitter
	result = s.HandleInput(400, y, true)

	if result != InputCaptured {
		t.Errorf("expected InputCaptured during drag, got %v", result)
	}
	if !s.dragging {
		t.Error("splitter should still be dragging when cursor moves below during drag")
	}
}

func TestSplitter_InputBounds(t *testing.T) {
	s := NewSplitter(600)
	s.Y = 300
	s.winW = 800

	bounds := s.InputBounds()

	if bounds.Min.X != 0 {
		t.Errorf("expected bounds.Min.X = 0, got %d", bounds.Min.X)
	}
	if bounds.Max.X != 800 {
		t.Errorf("expected bounds.Max.X = 800, got %d", bounds.Max.X)
	}
	// InputBounds must encompass BOTH the grab strip and the visible handle
	// pill, so the dispatcher routes presses on the drawn pill to HandleInput.
	// (No-steal of drum controls below the divider is enforced by HandleInput's
	// pill self-gate — see TestSplitterCapturesPillButNotOffPillBelowDivider.)
	grab := TouchGrabZone()
	gripStrip := image.Rect(0, s.Y-grab, s.winW, s.Y+4)
	pill := s.HandleRect().Inset(-SpaceSM)
	if bounds.Union(gripStrip) != bounds {
		t.Errorf("InputBounds %v must contain the grab strip %v", bounds, gripStrip)
	}
	if bounds.Union(pill) != bounds {
		t.Errorf("InputBounds %v must contain the visible handle pill %v", bounds, pill)
	}
}

func TestSplitter_ZIndex(t *testing.T) {
	s := NewSplitter(600)
	if s.ZIndex() != 150 {
		t.Errorf("expected ZIndex = 150, got %d", s.ZIndex())
	}
}

func TestSplitter_Capturing(t *testing.T) {
	s := NewSplitter(600)
	s.Y = 300
	s.winW = 800

	if s.Capturing() {
		t.Error("splitter should not be capturing initially")
	}

	// Start drag
	s.HandleInput(400, s.Y, true)
	if !s.Capturing() {
		t.Error("splitter should be capturing during drag")
	}

	// Release
	s.HandleInput(400, s.Y, false)
	if s.Capturing() {
		t.Error("splitter should not be capturing after release")
	}
}

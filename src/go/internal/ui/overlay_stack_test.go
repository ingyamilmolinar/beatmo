//go:build test

package ui

import (
	"image"
	"testing"
)

// mockOverlay is a test implementation of Overlay.
type mockOverlay struct {
	id        string
	bounds    image.Rectangle
	zIndex    int
	open      bool
	capturing bool
	handled   bool
	result    InputResult
	closed    bool
}

func (m *mockOverlay) ID() string                  { return m.id }
func (m *mockOverlay) InputBounds() image.Rectangle { return m.bounds }
func (m *mockOverlay) ZIndex() int                 { return m.zIndex }
func (m *mockOverlay) IsOpen() bool                { return m.open }
func (m *mockOverlay) Capturing() bool             { return m.capturing }

func (m *mockOverlay) Close() {
	m.open = false
	m.closed = true
}

func (m *mockOverlay) HandleInput(x, y int, pressed bool) InputResult {
	m.handled = true
	if !pressed {
		m.capturing = false
	}
	return m.result
}

func (m *mockOverlay) HandleWheel(x, y, steps int) InputResult {
	return InputIgnored
}

func TestOverlayStack_DispatchToTopOverlay(t *testing.T) {
	s := NewOverlayStack()

	bottom := &mockOverlay{
		id:     "bottom",
		bounds: image.Rect(0, 0, 100, 100),
		open:   true,
		result: InputConsumed,
	}
	top := &mockOverlay{
		id:     "top",
		bounds: image.Rect(0, 0, 100, 100),
		open:   true,
		result: InputConsumed,
	}

	s.Push(bottom)
	s.Push(top)

	result := s.HandleInput(50, 50, true)

	if result != InputConsumed {
		t.Errorf("expected InputConsumed, got %v", result)
	}
	if !top.handled {
		t.Error("top overlay should have been called")
	}
	if bottom.handled {
		t.Error("bottom overlay should not have been called when top consumed")
	}
}

func TestOverlayStack_SkipsClosedOverlays(t *testing.T) {
	s := NewOverlayStack()

	closed := &mockOverlay{
		id:     "closed",
		bounds: image.Rect(0, 0, 100, 100),
		open:   false, // closed
		result: InputConsumed,
	}
	open := &mockOverlay{
		id:     "open",
		bounds: image.Rect(0, 0, 100, 100),
		open:   true,
		result: InputConsumed,
	}

	s.Push(open)
	s.Push(closed) // closed overlay on top

	result := s.HandleInput(50, 50, true)

	if result != InputConsumed {
		t.Errorf("expected InputConsumed, got %v", result)
	}
	if closed.handled {
		t.Error("closed overlay should not have been called")
	}
	if !open.handled {
		t.Error("open overlay should have been called")
	}
}

func TestOverlayStack_CaptureMode(t *testing.T) {
	s := NewOverlayStack()

	overlay := &mockOverlay{
		id:        "capturer",
		bounds:    image.Rect(0, 0, 100, 100),
		open:      true,
		capturing: true,
		result:    InputCaptured,
	}

	s.Push(overlay)

	// First click captures
	result := s.HandleInput(50, 50, true)
	if result != InputCaptured {
		t.Errorf("expected InputCaptured, got %v", result)
	}
	if !s.Capturing() {
		t.Error("stack should be capturing after InputCaptured")
	}

	// Input outside bounds still goes to captured overlay
	overlay.handled = false
	result = s.HandleInput(200, 200, true)
	if result != InputCaptured {
		t.Errorf("expected InputCaptured for captured overlay, got %v", result)
	}
	if !overlay.handled {
		t.Error("captured overlay should receive input outside bounds")
	}

	// Release clears capture
	overlay.capturing = false
	s.HandleInput(200, 200, false)
	if s.Capturing() {
		t.Error("stack should not be capturing after release")
	}
}

func TestOverlayStack_ClickOutsideCloses(t *testing.T) {
	prev := suppressClicksUntilRelease
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = prev })

	s := NewOverlayStack()

	overlay := &mockOverlay{
		id:     "popup",
		bounds: image.Rect(50, 50, 150, 150),
		open:   true,
		result: InputConsumed,
	}

	s.Push(overlay)

	// Click outside bounds should close
	result := s.HandleInput(200, 200, true)
	if result != InputConsumed {
		t.Errorf("expected InputConsumed when closing, got %v", result)
	}
	if !overlay.closed {
		t.Error("overlay should be closed on click outside")
	}
	if overlay.open {
		t.Error("overlay should not be open after close")
	}
}

func TestOverlayStack_HasOpen(t *testing.T) {
	s := NewOverlayStack()

	if s.HasOpen() {
		t.Error("empty stack should not have open overlays")
	}

	closed := &mockOverlay{id: "closed", open: false}
	s.Push(closed)

	if s.HasOpen() {
		t.Error("stack with only closed overlays should not have open")
	}

	open := &mockOverlay{id: "open", open: true}
	s.Push(open)

	if !s.HasOpen() {
		t.Error("stack with open overlay should have open")
	}
}

func TestOverlayStack_CloseAll(t *testing.T) {
	s := NewOverlayStack()

	o1 := &mockOverlay{id: "1", open: true}
	o2 := &mockOverlay{id: "2", open: true, capturing: true}

	s.Push(o1)
	s.Push(o2)

	// Establish capture
	o2.result = InputCaptured
	s.HandleInput(50, 50, true)

	s.CloseAll()

	if o1.open || o2.open {
		t.Error("all overlays should be closed")
	}
	if s.Capturing() {
		t.Error("capture should be released after CloseAll")
	}
}

func TestOverlayStack_Clear(t *testing.T) {
	s := NewOverlayStack()

	o := &mockOverlay{id: "1", open: true, result: InputCaptured, capturing: true}
	s.Push(o)
	s.HandleInput(50, 50, true)

	s.Clear()

	if s.HasOpen() {
		t.Error("stack should be empty after clear")
	}
	if s.Capturing() {
		t.Error("capture should be released after clear")
	}
}

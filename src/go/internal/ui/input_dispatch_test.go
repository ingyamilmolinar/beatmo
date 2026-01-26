//go:build test

package ui

import (
	"image"
	"testing"
)

// mockHandler is a test implementation of InputHandler.
type mockHandler struct {
	bounds   image.Rectangle
	zIndex   int
	captured bool
	handled  bool
	result   InputResult
}

func (m *mockHandler) InputBounds() image.Rectangle { return m.bounds }
func (m *mockHandler) ZIndex() int             { return m.zIndex }
func (m *mockHandler) Capturing() bool         { return m.captured }

func (m *mockHandler) HandleInput(x, y int, pressed bool) InputResult {
	m.handled = true
	if !pressed {
		m.captured = false
	}
	return m.result
}

func (m *mockHandler) HandleWheel(x, y, steps int) InputResult {
	return InputIgnored
}

func TestInputDispatcher_ZOrdering(t *testing.T) {
	d := NewInputDispatcher()

	low := &mockHandler{
		bounds: image.Rect(0, 0, 100, 100),
		zIndex: 10,
		result: InputConsumed,
	}
	high := &mockHandler{
		bounds: image.Rect(0, 0, 100, 100),
		zIndex: 100,
		result: InputConsumed,
	}

	d.Register(low)
	d.Register(high)
	d.Sort()

	// Click in both handlers' bounds
	d.Dispatch(50, 50, true)

	if !high.handled {
		t.Error("high-z handler should have been called")
	}
	if low.handled {
		t.Error("low-z handler should not have been called when high-z consumed input")
	}
}

func TestInputDispatcher_CaptureMode(t *testing.T) {
	d := NewInputDispatcher()

	capturer := &mockHandler{
		bounds:   image.Rect(0, 0, 100, 100),
		zIndex:   50,
		result:   InputCaptured,
		captured: true,
	}

	d.Register(capturer)
	d.Sort()

	// First click - handler captures
	handled := d.Dispatch(50, 50, true)
	if !handled {
		t.Error("expected dispatch to return true when input captured")
	}

	// Now move outside bounds - should still receive input due to capture
	capturer.handled = false
	handled = d.Dispatch(200, 200, true)
	if !handled {
		t.Error("captured handler should receive input even outside bounds")
	}
	if !capturer.handled {
		t.Error("captured handler should have been called")
	}

	// Release - should clear capture
	capturer.captured = false // Simulate release
	d.Dispatch(200, 200, false)

	// Next click outside bounds should not reach handler
	capturer.handled = false
	handled = d.Dispatch(200, 200, true)
	if handled {
		t.Error("should not handle input outside bounds after capture released")
	}
}

func TestInputDispatcher_PropagationStopsOnConsumed(t *testing.T) {
	d := NewInputDispatcher()

	first := &mockHandler{
		bounds: image.Rect(0, 0, 100, 100),
		zIndex: 50,
		result: InputConsumed,
	}
	second := &mockHandler{
		bounds: image.Rect(0, 0, 100, 100),
		zIndex: 40,
		result: InputConsumed,
	}

	d.Register(first)
	d.Register(second)
	d.Sort()

	d.Dispatch(50, 50, true)

	if !first.handled {
		t.Error("first handler should have been called")
	}
	if second.handled {
		t.Error("second handler should not have been called after first consumed")
	}
}

func TestInputDispatcher_PropagationContinuesOnIgnored(t *testing.T) {
	d := NewInputDispatcher()

	first := &mockHandler{
		bounds: image.Rect(0, 0, 100, 100),
		zIndex: 50,
		result: InputIgnored,
	}
	second := &mockHandler{
		bounds: image.Rect(0, 0, 100, 100),
		zIndex: 40,
		result: InputConsumed,
	}

	d.Register(first)
	d.Register(second)
	d.Sort()

	d.Dispatch(50, 50, true)

	if !first.handled {
		t.Error("first handler should have been called")
	}
	if !second.handled {
		t.Error("second handler should have been called when first ignored")
	}
}

func TestInputDispatcher_OutOfBoundsIgnored(t *testing.T) {
	d := NewInputDispatcher()

	handler := &mockHandler{
		bounds: image.Rect(0, 0, 100, 100),
		zIndex: 50,
		result: InputConsumed,
	}

	d.Register(handler)
	d.Sort()

	handled := d.Dispatch(200, 200, true)

	if handled {
		t.Error("should not handle input outside all bounds")
	}
	if handler.handled {
		t.Error("handler should not have been called for out-of-bounds input")
	}
}

func TestInputDispatcher_Clear(t *testing.T) {
	d := NewInputDispatcher()

	handler := &mockHandler{
		bounds:   image.Rect(0, 0, 100, 100),
		zIndex:   50,
		result:   InputCaptured,
		captured: true,
	}

	d.Register(handler)
	d.Sort()
	d.Dispatch(50, 50, true) // Establish capture

	// Clear() should preserve capture for continued drags
	d.Clear()
	handler.handled = false
	handled := d.Dispatch(50, 50, true)
	if !handled {
		t.Error("capture should be preserved after Clear()")
	}
}

func TestInputDispatcher_ClearAll(t *testing.T) {
	d := NewInputDispatcher()

	handler := &mockHandler{
		bounds:   image.Rect(0, 0, 100, 100),
		zIndex:   50,
		result:   InputCaptured,
		captured: true,
	}

	d.Register(handler)
	d.Sort()
	d.Dispatch(50, 50, true) // Establish capture

	d.ClearAll()

	// After ClearAll, capture should be released and no handlers registered
	handler.handled = false
	handled := d.Dispatch(50, 50, true)
	if handled {
		t.Error("should not handle input after ClearAll")
	}
}

package ui

import (
	"image"
	"sort"
)

// InputDispatcher manages z-ordered input dispatch to UI components.
type InputDispatcher struct {
	handlers []InputHandler
	capture  InputHandler
}

// NewInputDispatcher creates a new input dispatcher.
func NewInputDispatcher() *InputDispatcher {
	return &InputDispatcher{}
}

// Register adds a handler to the dispatcher.
func (d *InputDispatcher) Register(h InputHandler) {
	d.handlers = append(d.handlers, h)
}

// Sort orders handlers by ZIndex (highest first for top-to-bottom dispatch).
func (d *InputDispatcher) Sort() {
	sort.SliceStable(d.handlers, func(i, j int) bool {
		return d.handlers[i].ZIndex() > d.handlers[j].ZIndex()
	})
}

// Dispatch sends input to handlers in z-order. Returns true if any handler
// consumed or captured the input.
func (d *InputDispatcher) Dispatch(x, y int, pressed bool) bool {
	// If a handler has captured input (e.g., during a drag), it receives
	// all input until it releases capture.
	if d.capture != nil {
		result := d.capture.HandleInput(x, y, pressed)
		if !d.capture.Capturing() {
			d.capture = nil
		}
		return result != InputIgnored
	}

	// Dispatch to handlers in z-order (highest first).
	pt := image.Pt(x, y)
	for _, h := range d.handlers {
		if !pt.In(h.InputBounds()) {
			continue
		}
		result := h.HandleInput(x, y, pressed)
		if result == InputCaptured {
			d.capture = h
			return true
		}
		if result == InputConsumed {
			return true
		}
	}
	return false
}

// Clear removes all registered handlers but preserves capture state.
// Use ClearAll to also release capture.
func (d *InputDispatcher) Clear() {
	d.handlers = nil
	// Note: capture is preserved so ongoing drags continue across frames
}

// ClearAll removes all handlers and releases any capture.
func (d *InputDispatcher) ClearAll() {
	d.handlers = nil
	d.capture = nil
}

package ui

import (
	"image"
	"sort"
)

// The handler that takes the press EDGE owns the whole gesture, so the grid's release-time
// tap and long-press must not also act beneath it.
type InputDispatcher struct {
	handlers []InputHandler
	capture  InputHandler

	wasPressed bool
	edge       bool
	// nil when the edge fell through every handler (the canvas owns the gesture).
	pressOwner InputHandler
}

// The opt-in for a handler that consumes its edge but still wants the grid's release gestures
// (the node sidebar resolves its deferred tap there).
type GesturePassthrough interface {
	PassesGesturesThrough() bool
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

func (d *InputDispatcher) Dispatch(x, y int, pressed bool) bool {
	edge := d.notePress(pressed)

	var declined InputHandler
	if d.capture != nil {
		h := d.capture
		result := h.HandleInput(x, y, pressed)
		if !h.Capturing() {
			d.capture = nil
		}
		if !(edge && result == InputIgnored) {
			if edge {
				d.pressOwner = h
			}
			return result != InputIgnored
		}
		// A capture holder that declines a NEW press has outlived its gesture (DrumView grabs one on a mere
		// hover while a dropdown is open): release it and dispatch normally, or the press has no owner.
		d.capture = nil
		declined = h
	}

	// Dispatch to handlers in z-order (highest first).
	pt := image.Pt(x, y)
	for _, h := range d.handlers {
		if h == declined || !pt.In(h.InputBounds()) {
			continue
		}
		result := h.HandleInput(x, y, pressed)
		if result == InputCaptured {
			d.capture = h
		}
		if result == InputCaptured || result == InputConsumed {
			if edge {
				d.pressOwner = h
			}
			return true
		}
	}
	return false
}

func (d *InputDispatcher) notePress(pressed bool) (edge bool) {
	edge = pressed && !d.wasPressed
	d.wasPressed = pressed
	d.edge = edge
	if edge {
		d.pressOwner = nil
	}
	return edge
}

// Keeps edge tracking honest on frames routed around the dispatcher, so a stale owner from an
// earlier gesture cannot swallow a new one.
func (d *InputDispatcher) SkipFrame(pressed bool) {
	d.notePress(pressed)
}

// Valid inside HandleInput.
func (d *InputDispatcher) PressEdge() bool { return d.edge }

func (d *InputDispatcher) PressOwner() InputHandler { return d.pressOwner }

// The press edge decides the owner once, wherever the finger lifts: otherwise a press that drifts
// off a button is also a tap on the grid.
func (d *InputDispatcher) GestureOwnedByHandler() bool {
	if d.pressOwner == nil {
		return false
	}
	if p, ok := d.pressOwner.(GesturePassthrough); ok && p.PassesGesturesThrough() {
		return false
	}
	return true
}

func (d *InputDispatcher) DispatchWheel(x, y, steps int) bool {
	if d.capture != nil {
		result := d.capture.HandleWheel(x, y, steps)
		return result != InputIgnored
	}
	pt := image.Pt(x, y)
	for _, h := range d.handlers {
		if !pt.In(h.InputBounds()) {
			continue
		}
		result := h.HandleWheel(x, y, steps)
		if result == InputConsumed || result == InputCaptured {
			return true
		}
	}
	return false
}

// Keeps capture; ClearAll releases it too.
func (d *InputDispatcher) Clear() {
	d.handlers = nil
	// Note: capture is preserved so ongoing drags continue across frames
}

// ClearAll removes all handlers and releases any capture.
func (d *InputDispatcher) ClearAll() {
	d.handlers = nil
	d.capture = nil
	d.pressOwner = nil
}

func (d *InputDispatcher) HasCaptureOtherThan(h InputHandler) bool {
	return d.capture != nil && d.capture != h
}

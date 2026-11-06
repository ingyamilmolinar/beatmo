package ui

import (
	"image"
	"sort"
)

// InputDispatcher manages z-ordered input dispatch to UI components.
//
// It also decides who OWNS each press cycle (press edge → release). The handler
// that consumes or captures the press EDGE owns the whole gesture: the
// release-time gestures the grid pane recognises separately (GestureTap and
// GestureLongPress, routed by Game.Update outside this dispatcher) must not
// also act on the canvas beneath it. See GestureOwnedByHandler.
type InputDispatcher struct {
	handlers []InputHandler
	capture  InputHandler

	// wasPressed is the pointer state of the previous frame (Dispatch or
	// SkipFrame), so a press EDGE can be told from a held frame.
	wasPressed bool
	// edge reports whether the frame being dispatched is a press edge, for
	// handlers that act only on the press that STARTS a gesture (PressEdge).
	edge bool
	// pressOwner is the handler that consumed/captured the current press
	// cycle's edge; nil when the edge fell through every handler (the canvas
	// owns the gesture) or no press cycle has started.
	pressOwner InputHandler
}

// GesturePassthrough is implemented by a handler that consumes its press edge
// but still wants the release-time grid gestures (GestureTap / long-press)
// delivered through Game's grid gesture path — the node sidebar resolves its
// deferred tap and scroll there. Swallowing is the default; passthrough is the
// explicit opt-in.
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

// Dispatch sends input to handlers in z-order. Returns true if any handler
// consumed or captured the input.
func (d *InputDispatcher) Dispatch(x, y int, pressed bool) bool {
	edge := d.notePress(pressed)

	// If a handler has captured input (e.g., during a drag), it receives
	// all input until it releases capture.
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
		// A capture holder that declines a NEW press does not own it: the
		// capture outlived its gesture (DrumView keeps one while a dropdown is
		// open, grabbed on a mere hover over the drum pane). Release it and
		// dispatch the press normally — otherwise a press elsewhere (the grid,
		// outside an open menu) reached no handler, had no owner, and its
		// release-time grid tap created a node.
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

// notePress tracks the pointer across frames and reports a press EDGE, which
// starts a new press cycle with no owner yet.
func (d *InputDispatcher) notePress(pressed bool) (edge bool) {
	edge = pressed && !d.wasPressed
	d.wasPressed = pressed
	d.edge = edge
	if edge {
		d.pressOwner = nil
	}
	return edge
}

// SkipFrame records the pointer state on a frame whose input is routed around
// the dispatcher (a touch gesture handled directly, an exclusive popup). It keeps
// the press-edge tracking honest: a press cycle that starts on such a frame has
// no dispatcher owner, so a stale owner from an earlier gesture cannot swallow it.
func (d *InputDispatcher) SkipFrame(pressed bool) {
	d.notePress(pressed)
}

// PressEdge reports whether the frame currently being dispatched starts a new
// press cycle. Valid inside HandleInput.
func (d *InputDispatcher) PressEdge() bool { return d.edge }

// PressOwner returns the handler that owns the current press cycle (it consumed
// or captured the press edge), or nil when the press fell through to the canvas.
func (d *InputDispatcher) PressOwner() InputHandler { return d.pressOwner }

// GestureOwnedByHandler reports whether the current press cycle belongs to a
// dispatcher handler that swallows its whole gesture, i.e. the release-time
// tap / long-press must NOT reach the canvas beneath. This is what makes a
// control drawn on top of the grid opaque for a whole gesture: the press edge
// decides the owner once, wherever the finger later lifts — without it a press
// on a button that drifts off it before lifting is ALSO a tap on the grid.
func (d *InputDispatcher) GestureOwnedByHandler() bool {
	if d.pressOwner == nil {
		return false
	}
	if p, ok := d.pressOwner.(GesturePassthrough); ok && p.PassesGesturesThrough() {
		return false
	}
	return true
}

// DispatchWheel sends a wheel event to handlers in z-order. If a handler
// has captured input, the wheel is routed to it. Otherwise, dispatch to the
// first handler whose InputBounds contains the point. Returns true if any
// handler consumed the wheel.
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
	d.pressOwner = nil
}

// HasCaptureOtherThan reports whether a handler other than h currently holds
// the dispatcher's capture. This lets components avoid starting new
// interactions when another component owns the input.
func (d *InputDispatcher) HasCaptureOtherThan(h InputHandler) bool {
	return d.capture != nil && d.capture != h
}

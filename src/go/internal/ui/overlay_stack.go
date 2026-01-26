package ui

import "image"

// Overlay represents a modal UI element that can capture input.
type Overlay interface {
	InputHandler
	ID() string
	IsOpen() bool
	Close()
}

// OverlayStack manages z-ordered modal overlays with capture semantics.
// When an overlay captures input (e.g., during scrollbar drag), it receives
// all input until capture is released. Clicking outside an open overlay
// closes it and suppresses click-through.
type OverlayStack struct {
	overlays []Overlay
	capture  Overlay
}

// NewOverlayStack creates a new overlay stack.
func NewOverlayStack() *OverlayStack {
	return &OverlayStack{}
}

// Push adds an overlay to the stack. Overlays are drawn/handled in order,
// with later overlays on top.
func (s *OverlayStack) Push(o Overlay) {
	s.overlays = append(s.overlays, o)
}

// HandleInput dispatches input to overlays in z-order (top to bottom).
// Returns the result of input handling.
func (s *OverlayStack) HandleInput(x, y int, pressed bool) InputResult {
	// If captured, route to capturing overlay
	if s.capture != nil {
		result := s.capture.HandleInput(x, y, pressed)
		if !s.capture.Capturing() {
			s.capture = nil
		}
		return result
	}

	// Dispatch top-to-bottom (reverse order since later overlays are on top)
	pt := image.Pt(x, y)
	for i := len(s.overlays) - 1; i >= 0; i-- {
		o := s.overlays[i]
		if !o.IsOpen() {
			continue
		}
		if pt.In(o.InputBounds()) {
			result := o.HandleInput(x, y, pressed)
			if result == InputCaptured {
				s.capture = o
			}
			if result != InputIgnored {
				return result
			}
		} else if pressed {
			// Click outside an open overlay closes it
			o.Close()
			SuppressClicksUntilMouseUp()
			return InputConsumed
		}
	}
	return InputIgnored
}

// Capturing returns true if any overlay is currently capturing input.
func (s *OverlayStack) Capturing() bool {
	return s.capture != nil
}

// HasOpen returns true if any overlay is currently open.
func (s *OverlayStack) HasOpen() bool {
	for _, o := range s.overlays {
		if o.IsOpen() {
			return true
		}
	}
	return false
}

// CloseAll closes all open overlays and releases capture.
func (s *OverlayStack) CloseAll() {
	for _, o := range s.overlays {
		if o.IsOpen() {
			o.Close()
		}
	}
	s.capture = nil
}

// Clear removes all overlays and releases capture.
func (s *OverlayStack) Clear() {
	s.overlays = nil
	s.capture = nil
}

// HandleWheel dispatches wheel events to overlays in z-order (top to bottom).
// Returns the result of input handling.
func (s *OverlayStack) HandleWheel(x, y, steps int) InputResult {
	if steps == 0 {
		return InputIgnored
	}

	// If captured, route to capturing overlay
	if s.capture != nil {
		return s.capture.HandleWheel(x, y, steps)
	}

	// Dispatch top-to-bottom (reverse order since later overlays are on top)
	pt := image.Pt(x, y)
	for i := len(s.overlays) - 1; i >= 0; i-- {
		o := s.overlays[i]
		if !o.IsOpen() {
			continue
		}
		if pt.In(o.InputBounds()) {
			return o.HandleWheel(x, y, steps)
		}
	}
	return InputIgnored
}

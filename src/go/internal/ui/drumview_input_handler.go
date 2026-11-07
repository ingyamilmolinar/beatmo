package ui

import "image"

// --- InputHandler interface implementation for DrumView ---

// Not Bounds(): DrumView.Bounds is a field.
func (dv *DrumView) InputBounds() image.Rectangle {
	return dv.Bounds
}

// Above base components, below the splitter.
func (dv *DrumView) ZIndex() int { return 100 }

func (dv *DrumView) HandleInput(x, y int, pressed bool) InputResult {
	// Touches can drop for one frame during a fast swipe, and clearing then would let the splitter
	// steal the drag; HandleInput runs before drum.Update, so TouchActive() is still true.
	if !pressed {
		if !dv.rowScroll().TouchActive() && !dv.anyDragActive() {
			dv.mouseDownInBounds = false
		}
	}

	// capturingDrag excludes overlays, so a portal never holds capture over the grid pane and
	// blocks camera panning.
	if dv.capturingDrag() {
		return InputCaptured
	}

	// A press that began here keeps capture, so the splitter cannot steal the drag.
	if dv.mouseDownInBounds && pressed {
		return InputCaptured
	}

	if !image.Pt(x, y).In(dv.Bounds) {
		return InputIgnored
	}

	// Capture so the tree's click-outside handling sees the press.
	if dv.anyDropdownOpen() {
		return InputCaptured
	}

	// Layout resize is now handled by the tree (layoutResizeZone at ZResize).

	// New press in drum view bounds — capture for duration of press.
	if pressed {
		dv.mouseDownInBounds = true
		return InputCaptured
	}

	return InputIgnored
}

// Note: Capturing() is already defined in drumview_notifications.go

func (dv *DrumView) HandleWheel(x, y, steps int) InputResult {
	if steps == 0 {
		return InputIgnored
	}

	// Check if point is within DrumView bounds
	if !image.Pt(x, y).In(dv.Bounds) {
		return InputIgnored
	}

	// Portal system handles overlay wheel dispatch.

	// Otherwise, let Update() handle the wheel event for row/zoom scrolling
	return InputIgnored
}

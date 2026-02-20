package ui

import "image"

// --- InputHandler interface implementation for DrumView ---

// InputBounds returns the interactive area of the DrumView.
// Note: We use InputBounds instead of Bounds() because DrumView.Bounds is a field.
func (dv *DrumView) InputBounds() image.Rectangle {
	return dv.Bounds
}

// ZIndex returns the DrumView's z-order for input dispatch.
// Higher than base components but lower than splitter.
func (dv *DrumView) ZIndex() int { return 100 }

// HandleInput processes mouse input for the DrumView.
// Portal system handles overlay input, then layout resize handler,
// then handles base DrumView input.
func (dv *DrumView) HandleInput(x, y int, pressed bool) InputResult {
	// Clear mouseDownInBounds on release, but only when no active touch
	// scroll or drag would lose capture. On mobile, touch events can
	// momentarily drop for a frame during fast swipes; clearing
	// unconditionally would release the dispatcher capture and let the
	// splitter steal the drag. HandleInput runs BEFORE drum.Update each
	// frame, so TouchActive() is still true on the flicker frame.
	if !pressed {
		if !dv.rowScroll().TouchActive() && !dv.anyDragActive() {
			dv.mouseDownInBounds = false
		}
	}

	// Active physical drag/scroll maintains capture regardless of position.
	// Use capturingDrag (excludes overlay state) to prevent portal entries
	// from holding the InputDispatcher capture when the touch moves to the
	// grid pane, which would block camera panning (panOK=false).
	if dv.capturingDrag() {
		return InputCaptured
	}

	// Press that started in drum view keeps capture for the entire press,
	// preventing splitter (or other handlers) from stealing mid-drag.
	if dv.mouseDownInBounds && pressed {
		return InputCaptured
	}

	if !image.Pt(x, y).In(dv.Bounds) {
		return InputIgnored
	}

	// When an overlay is open and the point is inside drum bounds,
	// capture so the tree's click-outside handling works correctly.
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

// HandleWheel processes wheel events for the DrumView.
// Portal system handles overlay dispatch, then falls through
// to allow DrumView.Update() to handle row/zoom scrolling.
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

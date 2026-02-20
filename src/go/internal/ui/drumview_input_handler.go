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
// Delegates to overlay stack for modal overlays, then layout resize handler,
// then handles base DrumView input.
func (dv *DrumView) HandleInput(x, y int, pressed bool) InputResult {
	// Clear mouseDownInBounds on release, but only when no active touch
	// scroll or drag would lose capture. On mobile, touch events can
	// momentarily drop for a frame during fast swipes; clearing
	// unconditionally would release the dispatcher capture and let the
	// splitter steal the drag. HandleInput runs BEFORE drum.Update each
	// frame, so TouchActive() is still true on the flicker frame.
	if !pressed {
		if !dv.rowScroll.TouchActive() && !dv.anyDragActive() {
			dv.mouseDownInBounds = false
		}
	}

	// Overlay stack must be dispatched BEFORE the Capturing() short-circuit
	// so that open overlays (context menu, overflow menu, etc.) can process
	// "click outside to close" events. Without this, Capturing() returns
	// true when any dropdown is open (via anyDropdownOpen), skipping the
	// overlay stack entirely and leaving overlays unable to close.
	if dv.overlays != nil && (image.Pt(x, y).In(dv.Bounds) || dv.overlays.Capturing()) {
		if result := dv.overlays.HandleInput(x, y, pressed); result != InputIgnored {
			return result
		}
		// When any overlay is open, block ALL below-overlay mouse input.
		// The OverlayStack is the single authority — no component below should
		// receive input while a modal is active.
		if pressed && dv.overlays.HasOpen() {
			return InputConsumed
		}
	}

	// Active drag/scroll maintains capture regardless of cursor position.
	if dv.Capturing() {
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

	// Check layout resize handler (non-modal but needs capture semantics)
	// Only check when no modal overlays are open
	if dv.layoutHandler != nil && !dv.anyDropdownOpen() {
		if result := dv.layoutHandler.HandleInput(x, y, pressed); result != InputIgnored {
			return result
		}
	}

	// New press in drum view bounds — capture for duration of press.
	if pressed {
		dv.mouseDownInBounds = true
		return InputCaptured
	}

	return InputIgnored
}

// Note: Capturing() is already defined in drumview_notifications.go

// HandleWheel processes wheel events for the DrumView.
// Delegates to overlay stack first for modal overlays, then falls through
// to allow DrumView.Update() to handle row/zoom scrolling.
func (dv *DrumView) HandleWheel(x, y, steps int) InputResult {
	if steps == 0 {
		return InputIgnored
	}

	// Check if point is within DrumView bounds
	if !image.Pt(x, y).In(dv.Bounds) {
		return InputIgnored
	}

	// Delegate to overlay stack first (handles modals like inst menu, eq channel menu)
	if dv.overlays != nil {
		if result := dv.overlays.HandleWheel(x, y, steps); result != InputIgnored {
			return result
		}
	}

	// Otherwise, let Update() handle the wheel event for row/zoom scrolling
	return InputIgnored
}

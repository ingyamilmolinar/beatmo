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
	// Check if point is within DrumView bounds
	if !image.Pt(x, y).In(dv.Bounds) {
		return InputIgnored
	}

	// Delegate to overlay stack first (handles modals like color wheel, inst menu)
	if dv.overlays != nil {
		if result := dv.overlays.HandleInput(x, y, pressed); result != InputIgnored {
			return result
		}
	}

	// Check layout resize handler (non-modal but needs capture semantics)
	// Only check when no modal overlays are open
	if dv.layoutHandler != nil && !dv.anyDropdownOpen() {
		if result := dv.layoutHandler.HandleInput(x, y, pressed); result != InputIgnored {
			return result
		}
	}

	// Check if any base control is capturing input
	if dv.Capturing() {
		return InputCaptured
	}

	// Click within DrumView bounds - consume to prevent click-through to grid
	if pressed && image.Pt(x, y).In(dv.Bounds) {
		return InputConsumed
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

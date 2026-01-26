package ui

import "image"

// RenameOverlay wraps the instrument rename dialog as an Overlay.
// It delegates to DrumView's existing rename logic while providing
// proper input isolation through the OverlayStack.
type RenameOverlay struct {
	dv *DrumView
}

func (o *RenameOverlay) ID() string { return "rename" }

func (o *RenameOverlay) IsOpen() bool {
	return o.dv.renameBox != nil
}

func (o *RenameOverlay) ZIndex() int { return 220 }

func (o *RenameOverlay) InputBounds() image.Rectangle {
	if o.dv.renameBox == nil {
		return image.Rectangle{}
	}
	return o.dv.renameBox.Rect
}

func (o *RenameOverlay) Capturing() bool {
	return o.dv.renameHold
}

func (o *RenameOverlay) Close() {
	o.dv.renameBox = nil
	o.dv.renameRow = -1
	o.dv.renameHold = false
	SuppressClicksUntilMouseUp()
}

func (o *RenameOverlay) HandleInput(x, y int, pressed bool) InputResult {
	// If renameHold is active, capture until release
	if o.dv.renameHold {
		if !pressed {
			o.dv.renameHold = false
		}
		return InputCaptured
	}

	// Point is within rename box - consume
	if o.dv.renameBox != nil && pressed && image.Pt(x, y).In(o.dv.renameBox.Rect) {
		return InputConsumed
	}

	return InputIgnored
}

func (o *RenameOverlay) HandleWheel(x, y, steps int) InputResult {
	// Consume but don't act - prevents pass-through to underlying elements
	return InputConsumed
}

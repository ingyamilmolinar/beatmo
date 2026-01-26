package ui

import "image"

// ColorWheelOverlay wraps the color picker wheel popup as an Overlay.
// It delegates to DrumView's existing color wheel logic while providing
// proper input isolation through the OverlayStack.
type ColorWheelOverlay struct {
	dv *DrumView
}

func (o *ColorWheelOverlay) ID() string { return "color-wheel" }

func (o *ColorWheelOverlay) IsOpen() bool {
	return o.dv.colorMenuOpen
}

func (o *ColorWheelOverlay) ZIndex() int { return 210 }

func (o *ColorWheelOverlay) InputBounds() image.Rectangle {
	return o.dv.colorWheelRect
}

func (o *ColorWheelOverlay) Capturing() bool {
	return o.dv.colorHold
}

func (o *ColorWheelOverlay) Close() {
	o.dv.colorMenuOpen = false
	o.dv.colorHold = false
	SuppressClicksUntilMouseUp()
}

func (o *ColorWheelOverlay) HandleInput(x, y int, pressed bool) InputResult {
	// If colorHold is active (just opened), capture until release
	if o.dv.colorHold {
		if !pressed {
			o.dv.colorHold = false
		}
		return InputCaptured
	}

	// Point is within color wheel bounds - consume
	if pressed && image.Pt(x, y).In(o.dv.colorWheelRect) {
		return InputConsumed
	}

	return InputIgnored
}

func (o *ColorWheelOverlay) HandleWheel(x, y, steps int) InputResult {
	// Consume but don't act - prevents pass-through to underlying elements
	return InputConsumed
}

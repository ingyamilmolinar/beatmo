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
	return o.dv.isColorMenuOpen()
}

func (o *ColorWheelOverlay) ZIndex() int { return 210 }

func (o *ColorWheelOverlay) InputBounds() image.Rectangle {
	if o.dv.colorWheelComp != nil && o.dv.colorWheelComp.IsOpen() {
		return o.dv.colorWheelComp.InputBounds()
	}
	return o.dv.colorWheelRect
}

func (o *ColorWheelOverlay) Capturing() bool {
	if o.dv.colorWheelComp != nil && o.dv.colorWheelComp.IsOpen() {
		return o.dv.colorWheelComp.Capturing()
	}
	return o.dv.colorHold
}

func (o *ColorWheelOverlay) Close() {
	if o.dv.colorWheelComp != nil {
		o.dv.colorWheelComp.Close()
	}
	o.dv.colorMenuOpen = false
	o.dv.colorHold = false
	SuppressClicksUntilMouseUp()
}

func (o *ColorWheelOverlay) HandleInput(x, y int, pressed bool) InputResult {
	// Delegate to the component for proper color picking
	if o.dv.colorWheelComp != nil && o.dv.colorWheelComp.IsOpen() {
		result := o.dv.colorWheelComp.HandleInput(x, y, pressed)
		// Sync legacy state
		o.dv.colorMenuOpen = o.dv.colorWheelComp.IsOpen()
		o.dv.colorHold = o.dv.colorWheelComp.Capturing()
		return result
	}

	// Legacy fallback
	if o.dv.colorHold {
		if !pressed {
			o.dv.colorHold = false
		}
		return InputCaptured
	}

	if pressed && image.Pt(x, y).In(o.dv.colorWheelRect) {
		return InputConsumed
	}

	return InputIgnored
}

func (o *ColorWheelOverlay) HandleWheel(x, y, steps int) InputResult {
	// Consume but don't act - prevents pass-through to underlying elements
	return InputConsumed
}

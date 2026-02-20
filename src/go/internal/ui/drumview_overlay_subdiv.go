package ui

import "image"

// SubdivMenuOverlay wraps the subdivisions dropdown as an Overlay.
// It delegates to DrumView's existing subdiv menu logic while providing
// proper input isolation through the OverlayStack.
type SubdivMenuOverlay struct {
	dv *DrumView
}

func (o *SubdivMenuOverlay) ID() string { return "subdiv-menu" }

func (o *SubdivMenuOverlay) IsOpen() bool {
	return o.dv.isSubdivMenuOpen()
}

func (o *SubdivMenuOverlay) ZIndex() int { return 200 }

func (o *SubdivMenuOverlay) InputBounds() image.Rectangle {
	if o.dv.subdivBtn == nil {
		return image.Rectangle{}
	}
	base := o.dv.subdivBtn.Rect()
	menuH := len(o.dv.subdivMenuBtns) * o.dv.rowHeight()
	return image.Rect(base.Min.X, base.Max.Y, base.Max.X, base.Max.Y+menuH)
}

func (o *SubdivMenuOverlay) Capturing() bool {
	return false // No drag/capture state in subdiv menu
}

func (o *SubdivMenuOverlay) Close() {
	o.dv.subdivMenuOpen = false
	SuppressClicksUntilMouseUp()
}

func (o *SubdivMenuOverlay) HandleInput(x, y int, pressed bool) InputResult {
	// Point is within menu bounds - consume
	if pressed && image.Pt(x, y).In(o.InputBounds()) {
		return InputConsumed
	}

	return InputIgnored
}

func (o *SubdivMenuOverlay) HandleWheel(x, y, steps int) InputResult {
	// Consume but don't act - prevents pass-through to underlying elements
	return InputConsumed
}

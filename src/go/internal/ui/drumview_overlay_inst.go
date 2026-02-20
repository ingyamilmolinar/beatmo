package ui

import "image"

// InstrumentMenuOverlay wraps the instrument selection dropdown as an Overlay.
// It delegates to DrumView's existing instrument menu logic while providing
// proper input isolation through the OverlayStack.
type InstrumentMenuOverlay struct {
	dv *DrumView
}

func (o *InstrumentMenuOverlay) ID() string { return "inst-menu" }

func (o *InstrumentMenuOverlay) IsOpen() bool {
	return o.dv.isInstMenuOpen()
}

func (o *InstrumentMenuOverlay) ZIndex() int { return 200 }

func (o *InstrumentMenuOverlay) InputBounds() image.Rectangle {
	if !o.dv.instMenuOpen {
		return image.Rectangle{}
	}

	// Use component bounds if available
	if o.dv.instMenuComp != nil && o.dv.instMenuComp.IsOpen() {
		compBounds := o.dv.instMenuComp.Bounds()
		if !compBounds.Empty() {
			// Also include the anchor label for toggle behavior
			if o.dv.instMenuRow >= 0 && o.dv.instMenuRow < len(o.dv.rowLabels) {
				label := o.dv.rowLabels[o.dv.instMenuRow].Rect()
				return image.Rect(
					min(label.Min.X, compBounds.Min.X),
					min(label.Min.Y, compBounds.Min.Y),
					max(label.Max.X, compBounds.Max.X),
					max(label.Max.Y, compBounds.Max.Y),
				)
			}
			return compBounds
		}
	}

	// Legacy fallback
	if o.dv.instMenuRow < 0 || o.dv.instMenuRow >= len(o.dv.rowLabels) {
		return image.Rectangle{}
	}
	label := o.dv.rowLabels[o.dv.instMenuRow].Rect()

	// Get the menu content rect
	menu := o.dv.instMenuFullRect
	if menu.Empty() {
		menu = o.dv.widgetRects[WidgetRack]
	}

	// Expand to include scrollbar area when present
	if o.dv.instMenuHasScroll() {
		menu.Max.X += instMenuScrollBarWidth
	}

	// Return union of label and menu bounds
	return image.Rect(
		min(label.Min.X, menu.Min.X),
		min(label.Min.Y, menu.Min.Y),
		max(label.Max.X, menu.Max.X),
		max(label.Max.Y, menu.Max.Y),
	)
}

func (o *InstrumentMenuOverlay) Capturing() bool {
	return o.dv.instMenuScroll.dragging || o.dv.instHold
}

func (o *InstrumentMenuOverlay) Close() {
	o.dv.instMenuOpen = false
	o.dv.instMenuScroll.EndDrag()
	o.dv.instHold = false
	SuppressClicksUntilMouseUp()
}

func (o *InstrumentMenuOverlay) HandleInput(x, y int, pressed bool) InputResult {
	// Only capture during active drag - let component handle all other input
	if o.dv.instMenuScroll.dragging {
		return InputCaptured
	}
	if o.dv.instHold {
		if !pressed {
			o.dv.instHold = false
		}
		return InputCaptured
	}

	// Detect click on the anchor label → toggle menu closed.
	// Skip while suppressClicksUntilRelease is active — the press that opened the
	// menu is still held, and toggling here would produce a 1-frame open/close flicker.
	if pressed && !suppressClicksUntilRelease && o.dv.instMenuRow >= 0 && o.dv.instMenuRow < len(o.dv.rowLabels) {
		lblRect := o.dv.rowLabels[o.dv.instMenuRow].Rect()
		if image.Pt(x, y).In(lblRect) {
			if o.dv.instMenuComp != nil && o.dv.instMenuComp.IsOpen() {
				o.dv.instMenuComp.Close()
			}
			o.Close()
			return InputConsumed
		}
	}

	// Return InputIgnored to let Update() delegate to component
	return InputIgnored
}

func (o *InstrumentMenuOverlay) HandleWheel(x, y, steps int) InputResult {
	if !o.dv.instMenuOpen {
		return InputIgnored
	}

	// Delegate to component if available
	if o.dv.instMenuComp != nil && o.dv.instMenuComp.IsOpen() {
		result := o.dv.instMenuComp.HandleWheel(x, y, steps)
		o.dv.syncInstMenuBtnsFromComp()
		return result
	}

	// Legacy fallback: scroll the menu
	if o.dv.instMenuScroll.ScrollBy(-steps) {
		o.dv.instMenuUserScrolled = true
		o.dv.buildInstMenu()
	}
	return InputConsumed // Always consume when menu is open and cursor is over it
}

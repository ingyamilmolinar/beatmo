package ui

import "image"

// ContextMenuOverlay wraps the mobile context menu as an Overlay.
// It delegates to DrumView's existing context menu logic while providing
// proper input isolation through the OverlayStack.
type ContextMenuOverlay struct {
	dv *DrumView
}

func (o *ContextMenuOverlay) ID() string { return "context-menu" }

func (o *ContextMenuOverlay) IsOpen() bool {
	return o.dv.contextMenuOpen
}

func (o *ContextMenuOverlay) ZIndex() int { return 220 }

func (o *ContextMenuOverlay) InputBounds() image.Rectangle {
	return o.dv.contextMenuRect
}

func (o *ContextMenuOverlay) Capturing() bool {
	if s := o.dv.contextMenuScroll; s != nil {
		if s.Dragging() || s.ScrollingCommitted() || s.TouchActive() {
			return true
		}
	}
	return o.dv.contextMenuDeferredTap.Active()
}

func (o *ContextMenuOverlay) Close() {
	o.dv.contextMenuOpen = false
	o.dv.contextMenuDeferredTap.Cancel()
	SuppressClicksUntilMouseUp()
}

func (o *ContextMenuOverlay) HandleInput(x, y int, pressed bool) InputResult {
	if o.dv.handleContextMenuInput(x, y, pressed) {
		if o.dv.contextMenuDeferredTap.Active() {
			return InputCaptured
		}
		if s := o.dv.contextMenuScroll; s != nil {
			if s.Dragging() || s.ScrollingCommitted() || s.TouchActive() {
				return InputCaptured
			}
		}
		return InputConsumed
	}
	return InputIgnored
}

func (o *ContextMenuOverlay) HandleWheel(x, y, steps int) InputResult {
	if o.dv.contextMenuScroll != nil && o.dv.contextMenuScroll.HasScroll() {
		o.dv.contextMenuScroll.HandleWheel(steps)
		o.dv.rebuildContextMenuButtons()
	}
	return InputConsumed // Prevent pass-through
}

// FXPanelOverlay wraps the FX insert effects panel as an Overlay.
// It delegates to DrumView's existing FX panel logic while providing
// proper input isolation through the OverlayStack.
type FXPanelOverlay struct {
	dv *DrumView
}

func (o *FXPanelOverlay) ID() string { return "fx-panel" }

func (o *FXPanelOverlay) IsOpen() bool {
	return o.dv.fxPanelOpen
}

func (o *FXPanelOverlay) ZIndex() int { return 210 }

func (o *FXPanelOverlay) InputBounds() image.Rectangle {
	r := o.dv.fxPanelRect
	// Include FX toggle buttons so clicking them doesn't trigger "click outside".
	for _, btn := range o.dv.rowFXBtns {
		if btn != nil && !btn.Rect().Empty() {
			r = r.Union(btn.Rect())
		}
	}
	return r
}

func (o *FXPanelOverlay) Capturing() bool {
	return o.dv.fxPanelDeferredTap.Active() || o.dv.fxScrollTS.Active() || o.dv.fxSliderDragging
}

func (o *FXPanelOverlay) Close() {
	o.dv.closeFXPanel()
	SuppressClicksUntilMouseUp()
}

func (o *FXPanelOverlay) HandleInput(x, y int, pressed bool) InputResult {
	// Check for FX button toggle click first.
	// Guard against re-triggering on the same press that opened the panel:
	// when suppressClicksUntilRelease is true, consume but don't toggle.
	if pressed {
		pt := image.Pt(x, y)
		for i, btn := range o.dv.rowFXBtns {
			if btn != nil && pt.In(btn.Rect()) {
				if !suppressClicksUntilRelease {
					o.dv.toggleFXPanel(i)
				}
				return InputConsumed
			}
		}
	}
	if o.dv.handleFXPanelInput(x, y, pressed) {
		if o.Capturing() {
			return InputCaptured
		}
		return InputConsumed
	}
	return InputIgnored
}

func (o *FXPanelOverlay) HandleWheel(x, y, steps int) InputResult {
	if o.dv.fxScrollMaxPx > 0 {
		o.dv.fxScrollOffsetPx -= steps * 20
		if o.dv.fxScrollOffsetPx < 0 {
			o.dv.fxScrollOffsetPx = 0
		}
		if o.dv.fxScrollOffsetPx > o.dv.fxScrollMaxPx {
			o.dv.fxScrollOffsetPx = o.dv.fxScrollMaxPx
		}
		o.dv.buildFXPanel()
	}
	return InputConsumed
}

// OverflowMenuOverlay wraps the mobile overflow menu (Upload/Import/Export) as
// an Overlay. It delegates to DrumView's existing overflow menu logic while
// providing proper input isolation through the OverlayStack.
type OverflowMenuOverlay struct {
	dv *DrumView
}

func (o *OverflowMenuOverlay) ID() string { return "overflow-menu" }

func (o *OverflowMenuOverlay) IsOpen() bool {
	return o.dv.overflowMenuOpen
}

func (o *OverflowMenuOverlay) ZIndex() int { return 230 }

func (o *OverflowMenuOverlay) InputBounds() image.Rectangle {
	popup := o.dv.overflowPopupRect()
	if o.dv.overflowBtn != nil {
		// Include the trigger button so clicking it doesn't count as "outside".
		btn := o.dv.overflowBtn.Rect()
		return popup.Union(btn)
	}
	return popup
}

func (o *OverflowMenuOverlay) Capturing() bool {
	if o.dv.overflowScroll != nil && o.dv.overflowScroll.Dragging() {
		return true
	}
	return o.dv.overflowDeferredTap.Active()
}

func (o *OverflowMenuOverlay) Close() {
	o.dv.closeOverflowMenu()
	SuppressClicksUntilMouseUp()
}

func (o *OverflowMenuOverlay) HandleInput(x, y int, pressed bool) InputResult {
	if o.dv.handleOverflowMenuInput(x, y, pressed) {
		if o.dv.overflowDeferredTap.Active() {
			return InputCaptured
		}
		return InputConsumed
	}
	return InputIgnored
}

func (o *OverflowMenuOverlay) HandleWheel(x, y, steps int) InputResult {
	if o.dv.overflowScroll != nil && o.dv.overflowScroll.HasScroll() {
		o.dv.overflowScroll.HandleWheel(steps)
	}
	return InputConsumed // Prevent pass-through
}

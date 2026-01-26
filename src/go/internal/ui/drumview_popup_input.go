package ui

import (
	"fmt"
	"image"
	"os"

	"github.com/hajimehoshi/ebiten/v2"
)

// handleInstMenuInput processes input for the instrument menu.
// Returns true if input was consumed and Update() should return early.
func (dv *DrumView) handleInstMenuInput(mx, my int, left bool) bool {
	if !dv.instMenuOpen {
		return false
	}

	// Determine menu bounds
	menuRect := dv.instMenuScroll.View
	if menuRect.Empty() {
		menuRect = dv.instMenuFullRect
	}
	if menuRect.Empty() {
		menuRect = dv.widgetRects[WidgetRack]
	}
	// Expand menu rect to include scrollbar area if present
	if dv.instMenuHasScroll() {
		menuRect.Max.X += instMenuScrollBarWidth
	}

	pt := image.Pt(mx, my)

	// Allow the Upload button to be clicked even when the menu is open.
	if left && dv.uploadBtn != nil && pt.In(dv.uploadBtn.Rect()) {
		dv.instMenuOpen = false
		dv.instMenuScroll.EndDrag()
		_ = dv.uploadBtn.Handle(mx, my, left)
		return true
	}

	// Handle category buttons when in categories mode
	if dv.instMenuMode == instMenuModeCategories {
		for _, btn := range dv.instCategoryBtns {
			if btn.Handle(mx, my, left) {
				return true
			}
		}
	}

	// NOTE: Wheel scrolling is now handled by InstrumentMenuOverlay.HandleWheel
	// via the overlay stack dispatch in DrumView.Update(). This ensures wheel
	// events are properly consumed before reaching row/zoom scroll handlers.

	// Scrollbar drag handling (must check BEFORE button clicks)
	if dv.instMenuHasScroll() {
		thumb := dv.instMenuThumbRect()
		if dv.instMenuScroll.dragging {
			if left {
				if dv.instMenuScroll.DragTo(my, instMenuScrollBarWidth, dv.rowHeight()/2) {
					dv.instMenuUserScrolled = true
					dv.buildInstMenu()
					dv.syncInstMenuCompScroll()
				}
			} else {
				dv.instMenuScroll.EndDrag()
			}
			return true
		}
		if left && pt.In(thumb) {
			_ = dv.instMenuScroll.StartDrag(my, instMenuScrollBarWidth, dv.rowHeight()/2)
			return true
		}
	}

	// Search box interaction.
	if dv.instMenuMode == instMenuModeInstruments && dv.instSearchBox != nil {
		prev := dv.instSearchBox.Value()
		consumed := dv.instSearchBox.Update()
		if consumed || dv.instSearchBox.Value() != prev {
			dv.instSearch = dv.instSearchBox.Value()
			dv.buildInstMenu()
			return true
		}
	}

	// Handle button clicks only when not dragging scrollbar.
	if !dv.instMenuScroll.dragging && dv.instMenuMode == instMenuModeInstruments {
		for _, btn := range dv.instMenuBtns {
			if btn.Handle(mx, my, left) {
				if os.Getenv("TUNKUL_DEBUG_INST") == "1" {
					fmt.Printf("[instMenu/click] %s\n", btn.Text)
				}
				// Back should keep the menu open and switch to categories.
				if btn.Text == "Back" {
					// OnClick has already rebuilt the menu; leave it open.
					dv.instMenuOpen = true
					dv.instMenuScroll.EndDrag()
					return true
				}
				dv.instMenuOpen = false
				dv.instMenuScroll.EndDrag()
				dv.instHold = true
				return true
			}
		}
	}

	// Click outside menu closes it
	if left {
		lbl := dv.rowLabels[dv.instMenuRow].Rect()
		menu := dv.instMenuFullRect
		if menu.Empty() {
			menu = dv.instMenuScroll.View
		}
		// Expand to include scrollbar
		if dv.instMenuHasScroll() {
			menu.Max.X += instMenuScrollBarWidth
		}
		if !pt.In(lbl) && !pt.In(menu) {
			dv.instMenuOpen = false
			dv.instMenuScroll.EndDrag()
			dv.instHold = true
		}
		return true
	}

	return true // Menu is open, consume all input to prevent click-through
}

// handleEQChannelMenuInput processes input for the EQ channel menu.
// Returns true if input was consumed and Update() should return early.
func (dv *DrumView) handleEQChannelMenuInput(mx, my int, left bool) bool {
	if !dv.eqChannelOpen {
		return false
	}

	menuRect := dv.eqChannelMenuRect()
	pt := image.Pt(mx, my)

	// Scrollbar drag continuation
	if dv.eqChannelScroll.dragging {
		if left {
			if dv.eqChannelScroll.DragTo(my, eqChannelMenuScrollBarWidth, dv.rowHeight()/2) {
				dv.buildEQChannelMenu()
			}
		} else {
			dv.eqChannelScroll.EndDrag()
		}
		return true
	}

	// NOTE: Wheel scrolling is now handled by EQChannelMenuOverlay.HandleWheel
	// via the overlay stack dispatch in DrumView.Update(). This ensures wheel
	// events are properly consumed before reaching row/zoom scroll handlers.

	// Scrollbar drag start
	if dv.eqChannelScroll.HasScroll() {
		thumb := dv.eqChannelScroll.ThumbRect(eqChannelMenuScrollBarWidth, dv.rowHeight()/2)
		if left && pt.In(thumb) {
			_ = dv.eqChannelScroll.StartDrag(my, eqChannelMenuScrollBarWidth, dv.rowHeight()/2)
			return true
		}
	}

	// Handle button clicks only when not dragging scrollbar
	if !dv.eqChannelScroll.dragging {
		for _, btn := range dv.eqChannelBtns {
			if btn.Handle(mx, my, left) {
				dv.eqChannelOpen = false
				dv.eqChannelScroll.EndDrag()
				dv.eqChannelScroll.First = 0
				return true
			}
		}
	}

	if left {
		// Respect click suppression to prevent immediate close after opening
		if suppressClicksUntilRelease {
			return true
		}
		base := dv.eqChannelBtn.Rect()
		menu := menuRect
		// Include scrollbar area in bounds check
		if dv.eqChannelScroll.HasScroll() {
			menu.Max.X += eqChannelMenuScrollBarWidth
		}
		if !pt.In(base) && !pt.In(menu) {
			dv.eqChannelOpen = false
			dv.eqChannelScroll.EndDrag()
			dv.eqChannelScroll.First = 0
		}
		return true
	}

	return true // Menu is open, consume all input
}

// handleColorMenuInput processes input for the legacy color menu.
// Returns true if input was consumed and Update() should return early.
func (dv *DrumView) handleColorMenuInput(mx, my int, left bool) bool {
	if !dv.colorMenuOpen {
		return false
	}

	// Handle Escape to close
	if !dv.colorHold && isKeyPressed(ebiten.KeyEscape) {
		dv.logger.Debugf("[COLOR] close by Esc")
		dv.colorMenuOpen = false
		return true
	}

	// Release colorHold once the mouse is released after opening to avoid
	// immediate close on the same press that opened the menu.
	if dv.colorHold {
		if !isMouseButtonPressed(ebiten.MouseButtonLeft) {
			dv.colorHold = false
			dv.logger.Debugf("[COLOR] hold released; wheel=%v", dv.colorWheelRect)
		}
	}

	pt := image.Pt(mx, my)

	// Handle clicks inside the color wheel: pick color at position.
	// Do not pick while colorHold is true (same press that opened the menu).
	if left && !dv.colorHold && pt.In(dv.colorWheelRect) {
		col := dv.pickColorFromWheel(mx, my)
		dv.SetRowColor(dv.colorMenuRow, col)
		dv.colorMenuOpen = false
		// Avoid click-through to underlying controls until release.
		SuppressClicksUntilMouseUp()
		dv.logger.Debugf("[COLOR] pick row=%d at=(%d,%d) wheel=%v sel=%s", dv.colorMenuRow, mx, my, dv.colorWheelRect, dv.colorKey(col))
		return true
	}

	if left && !dv.colorHold {
		// Click outside the wheel closes (ignore base button; it may be offscreen)
		if !pt.In(dv.colorWheelRect) {
			dv.logger.Debugf("[COLOR] close on outside click at=(%d,%d) wheel=%v", mx, my, dv.colorWheelRect)
			dv.colorMenuOpen = false
		}
	}

	return true // Menu is open, consume all input
}

// handleSubdivMenuInput processes input for the subdivision menu.
// Returns true if input was consumed and Update() should return early.
func (dv *DrumView) handleSubdivMenuInput(mx, my int, left bool) bool {
	if !dv.subdivMenuOpen {
		return false
	}

	// Legacy fallback path
	for _, btn := range dv.subdivMenuBtns {
		if btn.Handle(mx, my, left) {
			dv.subdivMenuOpen = false
			return true
		}
	}

	if left {
		base := dv.subdivBtn.Rect()
		menu := image.Rect(base.Min.X, base.Max.Y, base.Max.X, base.Max.Y+len(dv.subdivMenuBtns)*dv.rowHeight())
		pt := image.Pt(mx, my)
		if !pt.In(base) && !pt.In(menu) {
			dv.subdivMenuOpen = false
		}
		return true
	}

	return true // Menu is open, consume all input
}

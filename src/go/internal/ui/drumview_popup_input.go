package ui

import (
	"image"
)

// handleEQChannelMenuInput processes input for the EQ channel menu.
// Returns true if input was consumed and Update() should return early.
func (dv *DrumView) handleEQChannelMenuInput(mx, my int, left bool) bool {
	if !dv.eqChannelOpen {
		return false
	}

	menuRect := dv.eqChannelMenuRect()
	pt := image.Pt(mx, my)

	// Scrollbar drag continuation
	if dv.eqChannelScroll.Dragging() {
		if left {
			if dv.eqChannelScroll.HandleDragTo(my) {
				dv.buildEQChannelMenu()
			}
		} else {
			dv.eqChannelScroll.HandleDragEnd()
		}
		return true
	}

	// Touch scroll committed — process moves, clear deferred tap on end
	if dv.eqChannelScroll.ScrollingCommitted() {
		if left {
			if dv.eqChannelScroll.HandleTouchMove(mx, my) {
				dv.buildEQChannelMenu()
			}
		} else {
			dv.eqChDeferredTap.Cancel()
			dv.eqChannelScroll.HandleTouchEnd()
		}
		return true
	}

	// Touch active but not committed — continue tracking. On release,
	// check if it was a tap and fire deferred tap if so.
	if dv.eqChannelScroll.TouchActive() {
		if left {
			dv.eqChannelScroll.HandleTouchMove(mx, my)
		} else {
			wasTap := !dv.eqChannelScroll.ScrollingCommitted()
			dv.eqChannelScroll.HandleTouchEnd()
			if wasTap {
				dv.eqChDeferredTap.End(dv.fireEQChannelTapAt)
			} else {
				dv.eqChDeferredTap.Cancel()
			}
		}
		return true
	}

	// NOTE: Wheel scrolling is now handled by EQChannelMenuOverlay.HandleWheel
	// via the overlay stack dispatch in DrumView.Update(). This ensures wheel
	// events are properly consumed before reaching row/zoom scroll handlers.

	// Scrollbar drag start — check BEFORE touch begin because the thumb
	// is inside menuRect and would be intercepted as a touch scroll.
	if dv.eqChannelScroll.HasScroll() {
		thumb := dv.eqChannelScroll.ThumbRect()
		if left && pt.In(thumb) {
			dv.eqChannelScroll.HandleDragStart(my)
			return true
		}
	}

	// Touch begin in menu area: suppress ALL buttons on initial contact.
	// The deferred tap will fire on release if no scroll was committed.
	if left && pt.In(menuRect) && !dv.eqChannelScroll.TouchActive() && !dv.eqChannelScroll.Dragging() {
		if dv.eqChDeferredTap.Begin(mx, my) {
			dv.eqChannelScroll.HandleTouchBegin(mx, my)
			return true
		}
	}

	// Handle button clicks only when not touch-active and no deferred tap pending
	if !dv.eqChannelScroll.Dragging() && !dv.eqChannelScroll.TouchActive() && !dv.eqChDeferredTap.Active() {
		for _, btn := range dv.eqChannelBtns {
			if btn.Handle(mx, my, left) {
				dv.eqChannelOpen = false
				dv.eqChDeferredTap.Cancel()
				dv.eqChannelScroll.HandleDragEnd()
				dv.eqChannelScroll.ResetTouch()
				dv.eqChannelScroll.VS.First = 0
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
			dv.eqChDeferredTap.Cancel()
			dv.eqChannelScroll.HandleDragEnd()
			dv.eqChannelScroll.VS.First = 0
		}
		return true
	}

	return true // Menu is open, consume all input
}

// fireEQChannelTapAt finds the EQ channel button at (x, y) and calls its
// OnClick directly, bypassing Button.Handle's press-to-fire. Used for
// deferred taps where the touch has ended and we know it was a tap.
func (dv *DrumView) fireEQChannelTapAt(x, y int) {
	pt := image.Pt(x, y)
	for _, btn := range dv.eqChannelBtns {
		if pt.In(btn.Rect()) && btn.OnClick != nil {
			btn.OnClick()
			return
		}
	}
}

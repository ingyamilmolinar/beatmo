package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// mobileWheelPopupPortalOverlay wraps a *MobileWheelPopup as a PortalOverlay.
type mobileWheelPopupPortalOverlay struct {
	popup *MobileWheelPopup
	tag   string
}

func (o *mobileWheelPopupPortalOverlay) Layout(anchor, screenBounds image.Rectangle) {
	// MobileWheelPopup positions itself via Open(); nothing to do here.
}

func (o *mobileWheelPopupPortalOverlay) HitAreas() []HitArea {
	r := o.popup.Rect()
	if r.Empty() || !o.popup.IsOpen() {
		return nil
	}
	areas := []HitArea{
		{Rect: r, Handler: &mobileWheelHitHandler{popup: o.popup}, Tag: o.tag, Touch: true},
	}
	// Exact rect (no touch expansion) so it outranks the popup's expanded hit at overlapping pixels;
	// in the same modal overlay, so it survives the modal filter.
	if anchor := o.popup.Anchor(); !anchor.Empty() {
		areas = append(areas, HitArea{
			Rect:    anchor,
			Handler: &mobileWheelAnchorCloseHandler{popup: o.popup},
			Tag:     o.tag + "-anchor-close",
		})
	}
	return areas
}

func (o *mobileWheelPopupPortalOverlay) Draw(screen *ebiten.Image) { o.popup.Draw(screen) }
func (o *mobileWheelPopupPortalOverlay) ShouldClose() bool         { return !o.popup.IsOpen() }

func (o *mobileWheelPopupPortalOverlay) HoverAt(mx, my int) { o.popup.HoverAt(mx, my) }

// Cancels, then returns false so the universal CloseTop still removes the entry. Esc discards.
func (o *mobileWheelPopupPortalOverlay) HandleEscape() bool {
	o.popup.Cancel()
	return false
}

// Accepts (one undo step); the closed popup is cleaned up next frame.
func (o *mobileWheelPopupPortalOverlay) HandleEnter() bool {
	o.popup.Accept()
	return true
}

// mobileWheelHitHandler routes HitHandler events to a MobileWheelPopup.
type mobileWheelHitHandler struct{ popup *MobileWheelPopup }

func (h *mobileWheelHitHandler) OnPress(x, y int) InputResult {
	if h.popup.HandleInput(x, y, true) {
		if h.popup.IsDraggingAny() {
			return InputCaptured
		}
		return InputConsumed
	}
	return InputIgnored
}

func (h *mobileWheelHitHandler) OnDrag(x, y int)    { h.popup.HandleInput(x, y, true) }
func (h *mobileWheelHitHandler) OnRelease(x, y int) { h.popup.HandleInput(x, y, false) }
func (h *mobileWheelHitHandler) OnWheel(x, y, steps int) InputResult {
	h.popup.HandleWheel(x, y, steps) // scroll up/down adjusts value (or rung over the strip)
	return InputConsumed             // also prevents scroll-through to rows beneath
}

// The icon's own hit area is filtered out while the modal is open, so this lives inside the modal
// overlay to let a second press on the icon close it.
type mobileWheelAnchorCloseHandler struct{ popup *MobileWheelPopup }

func (h *mobileWheelAnchorCloseHandler) OnPress(x, y int) InputResult {
	if h.popup != nil && h.popup.IsOpen() {
		// Re-tapping the opener is a dismissal, not a cancel — persist the edit.
		h.popup.Accept()
	}
	return InputConsumed
}

func (h *mobileWheelAnchorCloseHandler) OnDrag(x, y int)    {}
func (h *mobileWheelAnchorCloseHandler) OnRelease(x, y int) {}
func (h *mobileWheelAnchorCloseHandler) OnWheel(x, y, steps int) InputResult {
	return InputIgnored
}

// Forwarded, so the arrows the popup handles are masked for the grid while it is open.
func (o *mobileWheelPopupPortalOverlay) ClaimedKeys() []ebiten.Key {
	return o.popup.ClaimedKeys()
}

func (o *mobileWheelPopupPortalOverlay) HandleClaimedKeys() bool {
	return o.popup.HandleClaimedKeys()
}

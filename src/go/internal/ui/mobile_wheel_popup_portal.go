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
	// Anchor tap-to-close: a second press on the icon that opened the popup
	// closes it. Exact-rect (no Touch expansion) so it outranks the popup's
	// touch-expanded hit at overlapping pixels (HitIndex sorts exact matches
	// first), and it belongs to the same modal overlay so it survives the
	// modal filter.
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

// HandleEscape (portalEscapeHandler) cancels the edit — reverting the value to
// the snapshot taken at Open — then returns false so handleEscape's universal
// CloseTop still removes the portal entry (and fires OnClose). Esc discards.
func (o *mobileWheelPopupPortalOverlay) HandleEscape() bool {
	o.popup.Cancel()
	return false
}

// HandleEnter (portalEnterHandler) accepts the edit — persisting it with one
// undo step — and closes. Returns true (consumed); the now-closed popup is
// removed by OverlayPortal.CleanupClosed next frame (ShouldClose()==true).
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

// mobileWheelAnchorCloseHandler closes the popup on press at the anchor icon
// rect. The icon's own (zone-owned) hit area is blocked by the modal filter
// while the popup is open; this handler lives inside the modal overlay's hit
// set so it survives that filter and gives the icon press a chance to toggle
// the popup closed.
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

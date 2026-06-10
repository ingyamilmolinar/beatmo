package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// sliderPopupPortalOverlay wraps a *SliderPopup as a PortalOverlay.
type sliderPopupPortalOverlay struct {
	popup *SliderPopup
	tag   string
}

func (o *sliderPopupPortalOverlay) Layout(anchor, screenBounds image.Rectangle) {
	// SliderPopup positions itself via Open(); nothing to do here.
}

func (o *sliderPopupPortalOverlay) HitAreas() []HitArea {
	r := o.popup.Rect()
	if r.Empty() {
		return nil
	}
	areas := []HitArea{
		{Rect: r, Handler: &sliderPopupHitHandler{popup: o.popup}, Tag: o.tag, Touch: true},
	}
	// Anchor tap-to-close: a second press on the icon that opened the popup
	// closes it. Exact-rect (no Touch expansion) so it outranks the popup's
	// touch-expanded hit at overlapping pixels (HitIndex sorts exact matches
	// first), and it belongs to the same modal overlay so it survives the
	// modal filter.
	if anchor := o.popup.Anchor(); !anchor.Empty() {
		areas = append(areas, HitArea{
			Rect:    anchor,
			Handler: &sliderPopupAnchorCloseHandler{popup: o.popup},
			Tag:     o.tag + "-anchor-close",
		})
	}
	return areas
}

func (o *sliderPopupPortalOverlay) Draw(screen *ebiten.Image) {
	o.popup.Draw(screen)
}

func (o *sliderPopupPortalOverlay) ShouldClose() bool {
	return !o.popup.IsOpen()
}

// sliderPopupHitHandler routes HitHandler events to a SliderPopup.
type sliderPopupHitHandler struct {
	popup *SliderPopup
}

func (h *sliderPopupHitHandler) OnPress(x, y int) InputResult {
	if h.popup.HandleInput(x, y, true) {
		if h.popup.IsDragging() {
			return InputCaptured
		}
		return InputConsumed
	}
	return InputIgnored
}

func (h *sliderPopupHitHandler) OnDrag(x, y int) {
	h.popup.HandleInput(x, y, true)
}

func (h *sliderPopupHitHandler) OnRelease(x, y int) {
	h.popup.HandleInput(x, y, false)
}

func (h *sliderPopupHitHandler) OnWheel(x, y, steps int) InputResult {
	return InputConsumed // prevent scroll-through
}

// sliderPopupAnchorCloseHandler closes the popup on press at the anchor
// icon rect. The icon's own (zone-owned) hit area is blocked by the modal
// filter while the popup is open; this handler lives inside the modal
// overlay's hit set so it survives that filter and gives the icon press a
// chance to toggle the popup closed.
type sliderPopupAnchorCloseHandler struct {
	popup *SliderPopup
}

func (h *sliderPopupAnchorCloseHandler) OnPress(x, y int) InputResult {
	if h.popup != nil && h.popup.IsOpen() {
		h.popup.Close()
	}
	return InputConsumed
}

func (h *sliderPopupAnchorCloseHandler) OnDrag(x, y int)    {}
func (h *sliderPopupAnchorCloseHandler) OnRelease(x, y int) {}
func (h *sliderPopupAnchorCloseHandler) OnWheel(x, y, steps int) InputResult {
	return InputIgnored
}

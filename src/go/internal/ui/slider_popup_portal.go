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
	// Exact-rect, so it outranks the popup's touch-expanded hit, and inside the modal overlay so it survives the filter.
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

// The icon's own hit area is blocked by the modal filter; this one lives inside the overlay.
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

// Up/Down nudge the slider instead of panning the grid while it is open.
func (o *sliderPopupPortalOverlay) ClaimedKeys() []ebiten.Key {
	return o.popup.ClaimedKeys()
}

func (o *sliderPopupPortalOverlay) HandleClaimedKeys() bool {
	return o.popup.HandleClaimedKeys()
}

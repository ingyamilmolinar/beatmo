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
	return []HitArea{
		{Rect: r, Handler: &sliderPopupHitHandler{popup: o.popup}, Tag: o.tag, Touch: true},
	}
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

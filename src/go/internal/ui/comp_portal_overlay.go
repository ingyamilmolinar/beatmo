package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// compOverlayable is the common interface implemented by overlay components
// (SubdivMenuComponent, InstrumentMenuComponent, ColorWheelComponent,
// RenameComponent). It is used by compPortalOverlay to wrap any of them
// as a PortalOverlay.
type compOverlayable interface {
	IsOpen() bool
	Close()
	InputBounds() image.Rectangle
	HandleInput(x, y int, pressed bool) InputResult
	HandleWheel(x, y, steps int) InputResult
	Capturing() bool
	Draw(dst *ebiten.Image)
}

// compPortalOverlay wraps a component as a PortalOverlay.
type compPortalOverlay struct {
	comp     compOverlayable
	tag      string
	updateFn func() // optional per-frame update (momentum, animation)
}

// Update implements PortalUpdater for per-frame momentum/animation updates.
func (o *compPortalOverlay) Update() {
	if o.updateFn != nil {
		o.updateFn()
	}
}

func (o *compPortalOverlay) Layout(anchor, screenBounds image.Rectangle) {
	// Components compute their own bounds via SetProps; nothing to do here.
}

func (o *compPortalOverlay) HitAreas() []HitArea {
	r := o.comp.InputBounds()
	if r.Empty() {
		return nil
	}
	return []HitArea{
		{Rect: r, Handler: &compHitHandler{comp: o.comp}, Tag: o.tag},
	}
}

func (o *compPortalOverlay) Draw(screen *ebiten.Image) {
	o.comp.Draw(screen)
}

func (o *compPortalOverlay) ShouldClose() bool {
	return !o.comp.IsOpen()
}

// compHitHandler routes HitHandler events to a compOverlayable.
type compHitHandler struct {
	comp compOverlayable
}

func (h *compHitHandler) OnPress(x, y int) InputResult {
	result := h.comp.HandleInput(x, y, true)
	if result == InputIgnored {
		return InputConsumed
	}
	return result
}

func (h *compHitHandler) OnDrag(x, y int) {
	h.comp.HandleInput(x, y, true)
}

func (h *compHitHandler) OnRelease(x, y int) {
	h.comp.HandleInput(x, y, false)
}

func (h *compHitHandler) OnWheel(x, y, steps int) InputResult {
	return h.comp.HandleWheel(x, y, steps)
}

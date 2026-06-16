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

// HandleEscape forwards Esc to the wrapped component if it implements
// portalEscapeHandler, so the universal Esc handler can let the component
// clear/cancel before the portal is closed.
func (o *compPortalOverlay) HandleEscape() bool {
	if h, ok := o.comp.(portalEscapeHandler); ok {
		return h.HandleEscape()
	}
	return false
}

// ClaimsKeyboard forwards to the wrapped component if it owns the keyboard while
// open (rename, instrument menu). Mirrors HandleEscape forwarding. Part of the
// keyboard-ownership contract (keyboard_focus.go).
func (o *compPortalOverlay) ClaimsKeyboard() bool {
	if c, ok := o.comp.(KeyboardClaimant); ok {
		return c.ClaimsKeyboard()
	}
	return false
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

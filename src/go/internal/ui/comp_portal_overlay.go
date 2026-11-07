package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

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

func (o *compPortalOverlay) HoverAt(mx, my int) {
	if h, ok := o.comp.(portalHoverer); ok {
		h.HoverAt(mx, my)
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

// The component gets to clear or cancel before the portal closes.
func (o *compPortalOverlay) HandleEscape() bool {
	if h, ok := o.comp.(portalEscapeHandler); ok {
		return h.HandleEscape()
	}
	return false
}

func (o *compPortalOverlay) ClaimsKeyboard() bool {
	if c, ok := o.comp.(KeyboardClaimant); ok {
		return c.ClaimsKeyboard()
	}
	return false
}

// A component without a claim claims nothing.
func (o *compPortalOverlay) ClaimedKeys() []ebiten.Key {
	if c, ok := o.comp.(KeyboardKeyClaimant); ok {
		return c.ClaimedKeys()
	}
	return nil
}

func (o *compPortalOverlay) HandleClaimedKeys() bool {
	if c, ok := o.comp.(KeyboardKeyClaimant); ok {
		return c.HandleClaimedKeys()
	}
	return false
}

// Not consumed unless the component handles it.
func (o *compPortalOverlay) HandleEnter() bool {
	if h, ok := o.comp.(portalEnterHandler); ok {
		return h.HandleEnter()
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

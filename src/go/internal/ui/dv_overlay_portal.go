package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// The SINGLE global portal, owned by the top-of-z overlay subtree, so every popup composites and hit-tests
// above every base zone.
func (dv *DrumView) portal() *OverlayPortal {
	if dv == nil || dv.overlayTree == nil {
		return nil
	}
	return dv.overlayTree.Portal()
}

type dvOverlayPortal struct {
	id       string
	isOpenFn func() bool
	rectFn   func() image.Rectangle
	inputFn  func(x, y int, pressed bool) InputResult
	wheelFn  func(x, y, steps int) InputResult
	drawFn   func(*ebiten.Image)
	updateFn func()           // optional per-frame update (momentum, animation)
	hoverFn  func(mx, my int) // optional per-frame plain-hover refresh (portalHoverer)
	escapeFn func() bool      // optional Esc handler; true → keep portal open
	// Required for any overlay hosting a focused text input, so the grid's shortcuts yield while it is open.
	claimsKeyboardFn func() bool
}

// Arrows then reach the caret, not the camera.
func (o *dvOverlayPortal) ClaimsKeyboard() bool {
	return o.claimsKeyboardFn != nil && o.claimsKeyboardFn()
}

// Update implements PortalUpdater for per-frame momentum/animation updates.
func (o *dvOverlayPortal) Update() {
	if o.updateFn != nil {
		o.updateFn()
	}
}

func (o *dvOverlayPortal) HoverAt(mx, my int) {
	if o.hoverFn != nil {
		o.hoverFn(mx, my)
	}
}

// Lets the overlay clear its own field (a search query) before the universal handler closes it.
func (o *dvOverlayPortal) HandleEscape() bool {
	if o.escapeFn != nil {
		return o.escapeFn()
	}
	return false
}

func (o *dvOverlayPortal) Layout(anchor, screenBounds image.Rectangle) {
	// DrumView-owned overlays compute their own rects.
}

func (o *dvOverlayPortal) HitAreas() []HitArea {
	r := o.rectFn()
	if r.Empty() {
		return nil
	}
	return []HitArea{
		{Rect: r, Handler: &dvOverlayHitHandler{overlay: o}, Tag: o.id},
	}
}

func (o *dvOverlayPortal) Draw(screen *ebiten.Image) {
	if o.drawFn != nil {
		o.drawFn(screen)
	}
}

func (o *dvOverlayPortal) ShouldClose() bool {
	return !o.isOpenFn()
}

// dvOverlayHitHandler routes HitHandler events to a dvOverlayPortal.
type dvOverlayHitHandler struct {
	overlay *dvOverlayPortal
}

func (h *dvOverlayHitHandler) OnPress(x, y int) InputResult {
	result := h.overlay.inputFn(x, y, true)
	if result == InputIgnored {
		return InputConsumed
	}
	return result
}

func (h *dvOverlayHitHandler) OnDrag(x, y int) {
	h.overlay.inputFn(x, y, true)
}

func (h *dvOverlayHitHandler) OnRelease(x, y int) {
	h.overlay.inputFn(x, y, false)
}

func (h *dvOverlayHitHandler) OnWheel(x, y, steps int) InputResult {
	if h.overlay.wheelFn != nil {
		return h.overlay.wheelFn(x, y, steps)
	}
	return InputConsumed
}

package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// portal returns the SINGLE global OverlayPortal, owned by the top-of-z
// overlay subtree. Every popup — whether opened by DrumView directly
// (drumview_portal_open.go) or by a zone via SetPortal — routes here, so all
// overlays composite and hit-test above every base zone in every subtree. This
// is the one place that answers "where do popups live?".
func (dv *DrumView) portal() *OverlayPortal {
	if dv == nil || dv.overlayTree == nil {
		return nil
	}
	return dv.overlayTree.Portal()
}

// dvOverlayPortal wraps a DrumView-owned overlay (ContextMenu, FXPanel,
// Overflow, Naming) as a PortalOverlay. Each instance delegates to existing
// DrumView methods via function fields.
type dvOverlayPortal struct {
	id       string
	isOpenFn func() bool
	rectFn   func() image.Rectangle
	inputFn  func(x, y int, pressed bool) InputResult
	wheelFn  func(x, y, steps int) InputResult
	drawFn   func(*ebiten.Image)
	updateFn func() // optional per-frame update (momentum, animation)
}

// Update implements PortalUpdater for per-frame momentum/animation updates.
func (o *dvOverlayPortal) Update() {
	if o.updateFn != nil {
		o.updateFn()
	}
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

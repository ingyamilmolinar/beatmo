package ui

import (
	"image"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

// An overlay, not Button.Draw: the dispatcher never tells a control it is merely hovered, and zones bake their
// chrome into caches. Once a frame it asks the HitIndex what is under the cursor and paints above every cache.

// An empty glowRect means no cushion: a latched or focused control shows its own state, or the cursor is in a
// gap. Drag-only surfaces do not implement it.
type glowTarget interface {
	glowRect(mx, my int, hitRect image.Rectangle) image.Rectangle
}

// Buttons — the button's live rect; empty (no cushion) while latched/active.
func (h *buttonHitAdapter) glowRect(_, _ int, _ image.Rectangle) image.Rectangle {
	if h.btn.Toggled() {
		return image.Rectangle{}
	}
	return h.btn.Rect()
}
func (h *repeatButtonHitAdapter) glowRect(_, _ int, _ image.Rectangle) image.Rectangle {
	if h.btn.Toggled() {
		return image.Rectangle{}
	}
	return h.btn.Rect()
}

// Empty while focused: the focus ring is the edit signal.
func (h *textInputHitAdapter) glowRect(_, _ int, _ image.Rectangle) image.Rectangle {
	if h.ti.Focused() {
		return image.Rectangle{}
	}
	return h.ti.Rect
}

// The adapter carries no rect, so the hit area's rect IS the control.
func (h *transportVolIconHitAdapter) glowRect(_, _ int, hitRect image.Rectangle) image.Rectangle {
	return hitRect
}
func (h *rowVolIconHitAdapter) glowRect(_, _ int, hitRect image.Rectangle) image.Rectangle {
	return hitRect
}

// The single slider under the cursor, not the whole column; empty between sliders.
func (h *sliderGroupHitAdapter) glowRect(mx, my int, _ image.Rectangle) image.Rectangle {
	if h.group == nil {
		return image.Rectangle{}
	}
	pt := image.Pt(mx, my)
	for _, s := range h.group.Sliders() {
		if r := s.Rect(); !r.Empty() && pt.In(r) {
			return r
		}
	}
	return image.Rectangle{}
}

// Portal hits are skipped (menus style their own hover); a suppressed topmost target yields nothing, never the
// control beneath it.
func (t *DrumViewTree) hoveredGlowRect(mx, my int) (image.Rectangle, bool) {
	for _, hit := range t.hitIndex.At(mx, my) {
		if hit.isPortal || hit.ZIndex >= ZOverlayMin {
			continue
		}
		gt, ok := hit.Handler.(glowTarget)
		if !ok {
			continue
		}
		if r := gt.glowRect(mx, my, hit.Rect); !r.Empty() {
			return r, true
		}
		return image.Rectangle{}, false
	}
	return image.Rectangle{}, false
}

// Desktop lifts on HOVER; touch has none, so mobile lifts the PRESSED control while held. Drawn over all zones,
// before the portal.
func (t *DrumViewTree) drawHoverGlow(screen *ebiten.Image) {
	target := 0.0
	mx, my := cursorPosition()
	if Profile().IsMobile() {
		if gt, ok := t.capturedHandler.(glowTarget); ok {
			if r := gt.glowRect(mx, my, t.hoverGlowRect); !r.Empty() {
				target = 1.0
				t.hoverGlowRect = r
			}
		}
	} else if r, ok := t.hoveredGlowRect(mx, my); ok {
		target = 1.0
		t.hoverGlowRect = r // snap so the glow slides between controls
	}

	// Allocation-free; settles exactly on the target.
	if t.hoverGlowAnim != target {
		a := genAnimButtonHoverFade
		t.hoverGlowAnim = target + (t.hoverGlowAnim-target)*a.Rate
		if math.Abs(t.hoverGlowAnim-target) < a.Threshold {
			t.hoverGlowAnim = target
		}
	}

	if t.hoverGlowAnim <= 0 || t.hoverGlowRect.Empty() {
		return
	}
	drawButtonHoverGlow(screen, t.hoverGlowRect, RadiusSM, t.hoverGlowAnim)
}

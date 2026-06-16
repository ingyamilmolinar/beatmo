package ui

import (
	"image"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

// hover_glow_overlay.go — the per-frame hover-glow affordance for clickable
// controls (buttons, text inputs like the BPM box, the volume icon/slider, …).
//
// Why an overlay (not Button.Draw): two facts make a per-control approach
// unworkable. (1) The input dispatcher only runs on press/drag/release, so a
// control never learns the cursor is merely *over* it — hover is never set on
// plain mouse-over for tree-dispatched controls (play/stop, instrument label,
// BPM box, …). (2) Zones render their chrome into state-hashed sprite caches
// (e.g. TransportZone.toolbarCache), so the control isn't redrawn per frame and
// a smooth fade can't render from inside the cache.
//
// The overlay sidesteps both: once per frame it asks the live HitIndex which
// clickable control is under the cursor (the index already knows every
// control's rect and z-order/occlusion), eases a single 0..1 fade toward "over
// a control?", and paints the cushion on top of everything — outside every
// cache. Desktop-only (DESIGN.md "mobile chrome stays flat"). See DESIGN.md
// § Cushioned elevation → Hover glow.

// glowTarget is implemented by hit-handler adapters for clickable controls that
// should show the hover cushion: buttons, text inputs (BPM box), the volume
// icon/slider, and similar. glowRect returns the visual rect to paint the
// cushion around for a cursor at (mx,my); hitRect is the hit area's own rect,
// used by controls that don't carry their own rect (the volume icon/popup
// trigger). An empty result means "no cushion": the control is suppressed (a
// latched/active button shows its own pulse; a focused input shows its focus
// ring) OR the cursor is over a gap inside a multi-control group. Drag-only
// surfaces (knobs, timeline scrub, grid drag, curve handles) do NOT implement
// it.
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
func (h *samplerButtonHitAdapter) glowRect(_, _ int, _ image.Rectangle) image.Rectangle {
	if h.btn.Toggled() {
		return image.Rectangle{}
	}
	return h.btn.Rect()
}

// Text inputs (BPM box, …) — the input's rect; empty while focused (the focus
// ring is the active-edit signal there).
func (h *textInputHitAdapter) glowRect(_, _ int, _ image.Rectangle) image.Rectangle {
	if h.ti.Focused() {
		return image.Rectangle{}
	}
	return h.ti.Rect
}

// Volume icon/popup triggers — the adapter carries no control rect, so the hit
// area's rect IS the control.
func (h *transportVolIconHitAdapter) glowRect(_, _ int, hitRect image.Rectangle) image.Rectangle {
	return hitRect
}
func (h *rowVolIconHitAdapter) glowRect(_, _ int, hitRect image.Rectangle) image.Rectangle {
	return hitRect
}

// Slider groups (master volume, the per-row volume column, …) — resolve to the
// INDIVIDUAL slider under the cursor so each row's volume glows independently
// rather than as the whole grouped column. Empty when the cursor is between
// sliders.
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

// hoveredGlowRect returns the visual rect of the topmost clickable control
// under the cursor that should show the hover cushion, and whether one was
// found. Portal/overlay hits (menus, dialogs) are skipped — those carry their
// own hover styling (menu_chrome). A suppressed/gap topmost target yields no
// glow (we never glow a control beneath the one actually under the cursor).
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

// drawHoverGlow advances the hover-fade one frame and strokes the glow ring
// around the hovered button. Called from Draw, on top of all zones, before the
// portal. Desktop-only; a no-op (and self-clearing) on mobile.
func (t *DrumViewTree) drawHoverGlow(screen *ebiten.Image) {
	if Profile().IsMobile() {
		t.hoverGlowAnim = 0
		t.hoverGlowRect = image.Rectangle{}
		return
	}

	target := 0.0
	mx, my := cursorPosition()
	if r, ok := t.hoveredGlowRect(mx, my); ok {
		target = 1.0
		t.hoverGlowRect = r // snap so the glow slides between controls
	}

	// Ease the 0..1 progress toward the target via the button-hover-fade
	// spring (allocation-free; settles exactly on the target).
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

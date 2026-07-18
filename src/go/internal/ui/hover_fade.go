package ui

import "math"

// hoverFade is the ONE shared hover animator for surfaces that live outside
// the DrumViewTree overlay (node sidebar section headers/action rows, grid
// nodes, …). It tracks WHICH control is hovered (caller-chosen id) and eases
// a 0..1 lift progress with the same button-hover-fade spring the tree's
// hover overlay uses (hover_glow_overlay.go), so every hover affordance in
// the app glides identically instead of each surface hand-rolling a fade.
// Zero-value ready; allocation-free per frame.
type hoverFade struct {
	id     string  // hovered control this frame ("" = none)
	lastID string  // last non-empty id — the surface the fade-out renders on
	p      float64 // eased 0..1 progress
}

// Advance records the hovered control for this frame ("" when the cursor is
// over nothing hoverable) and eases the progress one step toward it.
// Jumping directly between two controls restarts the ease so the lift
// re-blooms on the new control; hover-out fades the last control back down.
// Returns the current progress.
func (h *hoverFade) Advance(id string) float64 {
	if id != "" && id != h.lastID {
		h.p = 0 // new control: re-bloom from zero
		h.lastID = id
	}
	h.id = id
	target := 0.0
	if h.id != "" {
		target = 1.0
	}
	if h.p != target {
		a := genAnimButtonHoverFade
		h.p = target + (h.p-target)*a.Rate
		if math.Abs(h.p-target) < a.Threshold {
			h.p = target
		}
	}
	return h.p
}

// Lift reports the current progress for the given control id: the eased
// progress while it is the (fading) hover target, 0 for every other control.
// Call from the draw pass: drawButtonHoverGlow(dst, r, rad, h.Lift(id)).
func (h *hoverFade) Lift(id string) float64 {
	if id != h.lastID {
		return 0
	}
	return h.p
}

// advanceNodeHoverGlow eases the grid-node hover halo toward the node under
// the cursor (g.hover) with the shared button-hover-fade spring — the same
// glide every other hover affordance uses. Runs once per Update, right
// after g.hover is computed; allocation-free.
func (g *Game) advanceNodeHoverGlow() {
	if g.hover != nil && g.hover != g.hoverGlowNode {
		g.hoverGlowNode = g.hover // new node: re-bloom from zero
		g.hoverGlowP = 0
	}
	target := 0.0
	if g.hover != nil {
		target = 1
	}
	if g.hoverGlowP != target {
		a := genAnimButtonHoverFade
		g.hoverGlowP = target + (g.hoverGlowP-target)*a.Rate
		if math.Abs(g.hoverGlowP-target) < a.Threshold {
			g.hoverGlowP = target
		}
	}
	if g.hover == nil && g.hoverGlowP == 0 {
		g.hoverGlowNode = nil
	}
}

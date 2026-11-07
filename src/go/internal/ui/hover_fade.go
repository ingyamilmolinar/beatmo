package ui

import "math"

// The ONE hover animator for surfaces outside the tree's overlay, on the same spring, so every hover glides alike.
// Zero value ready; allocation-free.
type hoverFade struct {
	id     string  // hovered control this frame ("" = none)
	lastID string  // last non-empty id — the surface the fade-out renders on
	p      float64 // eased 0..1 progress
}

// Jumping between controls restarts the ease so the lift re-blooms; hover-out fades the last one down.
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

// 0 for every control but the (fading) target.
func (h *hoverFade) Lift(id string) float64 {
	if id != h.lastID {
		return 0
	}
	return h.p
}

// Runs once per Update, right after g.hover is computed.
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

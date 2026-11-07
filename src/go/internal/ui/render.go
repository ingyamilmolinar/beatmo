package ui

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
)

// Render reads one global, Profile().DrawTopEdgeHighlight, hoisted at entry; everything else comes from
// spec/state. It must stay allocation-free per call.

// Disabled overrides hover/press/focus; the caller passes the pre-resolved disabled spec.
func Render(dst *ebiten.Image, r image.Rectangle, spec ComponentSpec, state ComponentState) {
	// Flat-tonal: one filled body, no border or bevel; hover/press are fill deltas and the focus ring is the
	// only outline. spec.Border is ignored here, not deleted, so the YAML recipes stay readable.
	var fill color.Color = spec.Fill
	switch {
	case state.Disabled:
		fill = colSurface1
	default:
		if state.Hovered && !state.Pressed {
			if d := spec.Interaction.Hover.FillDelta; d != 0 {
				fill = adjustColor(fill, int(d))
			}
		}
		if state.Focused {
			if d := spec.Interaction.Focus.FillDelta; d != 0 {
				fill = adjustColor(fill, int(d))
			}
		}
		if state.Pressed {
			fill = adjustColor(fill, -20)
		}
	}
	drawFlatButtonBody(dst, r, fill, spec.Radius)
	if state.Focused {
		// AlphaStrong so it shows against every secondary surface tone.
		drawRect(dst, r.Inset(1), WithAlpha(genColorFocusRing, genAlphaStrong), false)
	}
}

// renderLegacy takes raw colors so the hand-coded *Style adapters route through the same primitive,
// byte-for-byte.
func renderLegacy(dst *ebiten.Image, r image.Rectangle, fill, border color.Color, hover, focus InteractionDelta, state ComponentState) {
	topEdge := Profile().DrawTopEdgeHighlight
	if state.Disabled {
		drawButton(dst, r, fill, border, false, topEdge)
		return
	}
	if state.Hovered && !state.Pressed {
		if d := hover.FillDelta; d != 0 {
			fill = adjustColor(fill, int(d))
		}
		if d := hover.BorderDelta; d != 0 {
			border = adjustColor(border, int(d))
		}
	}
	if state.Focused {
		if d := focus.FillDelta; d != 0 {
			fill = adjustColor(fill, int(d))
		}
		if d := focus.BorderDelta; d != 0 {
			border = adjustColor(border, int(d))
		}
	}
	drawButton(dst, r, fill, border, state.Pressed, topEdge)
	if state.Focused {
		drawRect(dst, r.Inset(1), WithAlpha(genColorFocusRing, genAlphaStrong), false)
	}
}

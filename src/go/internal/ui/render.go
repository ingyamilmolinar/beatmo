package ui

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
)

// render.go — Phase 4 of the design-system refactor.
//
// Render is the chrome-rendering primitive: given a ComponentSpec (the
// immutable visual recipe from DESIGN.md → codegen) and a ComponentState
// (the per-frame variable input), it draws the component at the given
// rectangle.
//
// Purity boundary: Render reads exactly one global at function entry —
// Profile().DrawTopEdgeHighlight, the desktop-vs-mobile depth-accent
// rule. That bool is hoisted into a local and threaded through to
// drawButton (which is otherwise pure w.r.t. its parameters). All other
// behaviour comes from spec/state — no other Profile()/Theme() reads
// happen below the entry. The single-flag concession is documented here
// so future readers don't trust the comment over the code; the proper
// next step is to make Spec(ID) profile-aware (so the bool lives on
// ComponentSpec for the active profile and Render reads zero globals).
//
// The function aims to be allocation-free per call (asserted, not just
// reported, by render_bench_test.go). All color values flow through
// value-typed color.RGBA / color.NRGBA structs; the only interface
// boxing happens when calling drawButton / drawRect (which take
// color.Color).

// Render draws the chrome surface described by spec, applying state-based
// hover / press / focus deltas, the optional focus accent ring, and the
// component's borders. Behaviour matches the legacy ButtonStyle.Draw +
// TextInputStyle.DrawAnimated paths; Phase 4 PR2 delegates them here.
//
// Disabled overrides hover/press/focus: the disabled spec (looked up by
// the caller) is drawn flat, no interaction deltas applied. Phase 4 PR1
// does not wire automatic variant lookup — callers must pass a
// pre-resolved disabled spec when state.Disabled is true.
func Render(dst *ebiten.Image, r image.Rectangle, spec ComponentSpec, state ComponentState) {
	topEdge := Profile().DrawTopEdgeHighlight
	// Fast-path zero-delta case: identical work to the legacy
	// ButtonStyle.Draw, identical allocation count. The state checks
	// below are short-circuited so spec entries with no Interaction
	// (panels, decorations, dynamic widgets) never pay for unused logic.
	if state.Disabled || (!state.Hovered && !state.Focused) {
		drawButton(dst, r, spec.Fill, spec.Border.Resolve(), state.Pressed, topEdge)
		return
	}

	// Slow path: at least one interaction delta needs applying.
	var fill color.Color = spec.Fill
	var border color.Color = spec.Border.Resolve()
	if state.Hovered && !state.Pressed {
		if d := spec.Interaction.Hover.FillDelta; d != 0 {
			fill = adjustColor(fill, int(d))
		}
		if d := spec.Interaction.Hover.BorderDelta; d != 0 {
			border = adjustColor(border, int(d))
		}
	}
	if state.Focused {
		if d := spec.Interaction.Focus.FillDelta; d != 0 {
			fill = adjustColor(fill, int(d))
		}
		if d := spec.Interaction.Focus.BorderDelta; d != 0 {
			border = adjustColor(border, int(d))
		}
	}
	drawButton(dst, r, fill, border, state.Pressed, topEdge)
	if state.Focused {
		// Focus accent ring inside the rect — matches the TextInputStyle
		// focused-input look. Drawn at AlphaStrong over the focus-ring
		// token so it's visible against any of the secondary surface tones.
		drawRect(dst, r.Inset(1), WithAlpha(genColorFocusRing, genAlphaStrong), false)
	}
}

// renderLegacy is a back-door used by the *Style adapters in components.go
// during Phase 4 PR2. It accepts the existing color.Color / interaction
// values rather than requiring a full ComponentSpec — preserving exact
// byte-for-byte behavior of the hand-coded ButtonStyle.Draw etc. while
// routing all chrome rendering through a single primitive.
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

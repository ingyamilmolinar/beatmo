//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestDrawSynthFocusGraph_DrawsSelectedKnobDomain — the focus graph paints the
// property-native picture for the SELECTED knob: with the synth tab open on a
// resolved instrument and a section selected, drawSynthFocusGraph must produce
// ink INSIDE the inner graph region (i.e. beyond the outer chrome frame), so a
// knob the user picked actually gets a graph drawn for it.
func TestDrawSynthFocusGraph_DrawsSelectedKnobDomain(t *testing.T) {
	g := newModularSynthTabGame(t)
	layoutSynthTab(t, g)
	dv := g.drum

	inst := dv.resolveSynthInstrument(dv.synthTabActiveInstrument())
	if inst == "" {
		t.Skip("no synth instrument")
	}
	sec := dv.synthSelectedSection()
	if sec == nil || len(sec.knobIdxs) == 0 {
		t.Skip("no open section with knobs")
	}
	dv.setSynthSelectedKnob(inst, sec.knobIdxs[0])

	const w, h = 300, 130
	rect := image.Rect(0, 0, w, h)
	rects := collectFilledRects(t, func() {
		dv.drawSynthFocusGraph(ebiten.NewImage(w, h), rect, inst)
	})
	if countNonBgRects(rects) == 0 {
		t.Fatalf("focus graph painted nothing for selected knob")
	}

	// Stronger: ink must land inside the inner graph band, not only the outer
	// chrome frame — proves the renderer (not just the surface fill) drew.
	pad := SpaceSM
	captionH := int(float64(TextHeight()) * (FontSizeCaption / FontSizeBody))
	inner := image.Rect(rect.Min.X+pad, rect.Min.Y+pad+captionH, rect.Max.X-pad, rect.Max.Y-pad).Inset(2)
	innerInk := 0
	for _, r := range rects {
		if r.Rect.Overlaps(inner) {
			innerInk++
		}
	}
	if innerInk == 0 {
		t.Fatalf("focus graph drew no ink inside the inner graph region %v", inner)
	}
}

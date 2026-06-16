//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// TestEveryKnobHasConceptInk guards the requirement that EVERY synth knob shows
// a graphic — at its DEFAULT value, with no stage enabled. A renderer that
// draws only the scope background (e.g. FM bars when all depths are zero) leaves
// a knob with a blank band, which reads as "broken". Every knob's concept
// renderer must put at least some non-background ink in the band.
func TestEveryKnobHasConceptInk(t *testing.T) {
	g := setupModularSynthGame(t)
	bindings := g.drum.SynthTabBindings()
	if len(bindings) == 0 {
		t.Fatal("no synth bindings to check")
	}
	dst := ebiten.NewImage(160, 60)
	r := image.Rect(0, 0, 160, 60)
	for _, b := range bindings {
		group := b.def.Group
		rend := conceptRendererForGroup(group)
		rects := collectFilledRects(t, func() { rend(dst, r, "modular", b.def, nil) })
		if countNonBgRects(rects) == 0 {
			t.Errorf("knob %q (group %q) drew NO concept graphic at its default value", b.def.Name, group)
		}
	}
}

// TestConceptKnobWave_RendersAndMorphs proves each knob renders its OWN wave
// that reflects the resulting signal: the filter-cutoff knob draws a wave, and
// that wave is DIFFERENT at a bright vs a dull cutoff (data-driven, not static).
func TestConceptKnobWave_RendersAndMorphs(t *testing.T) {
	const inst = "concept-knob-wave"
	audio.BindInstrumentToRecipe(inst, "synth-modular")
	t.Cleanup(func() { audio.ResetInstrumentParams(inst) })
	audio.SetInstrumentParam(inst, "osc_type", 1) // saw — harmonic rich, so the filter visibly bites
	def := audio.ParamDef{Name: "filter_cutoff", Group: "filter", Min: 20, Max: 20000}

	dst := ebiten.NewImage(160, 60)
	r := image.Rect(0, 0, 160, 60)
	multiset := func(rs []drawnRect) map[image.Rectangle]int {
		m := map[image.Rectangle]int{}
		for _, x := range rs {
			m[x.Rect]++
		}
		return m
	}

	audio.SetInstrumentParam(inst, "filter_cutoff", 9000)
	hi := collectFilledRects(t, func() { conceptKnobWave(dst, r, inst, def, nil) })
	if countNonBgRects(hi) == 0 {
		t.Fatal("conceptKnobWave drew no wave")
	}
	audio.SetInstrumentParam(inst, "filter_cutoff", 150)
	lo := collectFilledRects(t, func() { conceptKnobWave(dst, r, inst, def, nil) })
	if countNonBgRects(lo) == 0 {
		t.Fatal("conceptKnobWave drew no wave at low cutoff")
	}
	if mapsEqualRectCount(multiset(hi), multiset(lo)) {
		t.Error("conceptKnobWave wave identical for bright vs dull cutoff — not reflecting the knob")
	}
}

// TestSynthCellHasNoVizBand guards the post-Task-8 layout: the per-knob
// concept-viz band is gone (the focus graph in the right pane explains the
// selected knob), so the knob cell reserves NO band below the dial+caption.
// The knob fills the cell (dial + caption span the whole cell height) instead
// of yielding room for a mini-picture.
func TestSynthCellHasNoVizBand(t *testing.T) {
	g := setupModularSynthGame(t)
	expandSynthPanelForTest(t, g)
	// Give the view plenty of height so the synth detail pane is not clamped.
	g.Layout(1280, 1600)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())

	secID, _ := synthSectionContaining(t, g, "filter")
	g.drum.setSelectedSynthSection("modular", secID)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())

	grid := g.drum.sectionGrid(secID)
	cell, vis := grid.CellRect(0)
	if !vis {
		t.Fatalf("first knob cell of section %v not visible", secID)
	}
	knobIdx := -1
	for _, s := range g.drum.SynthTabSections() {
		if s.id == secID && len(s.knobIdxs) > 0 {
			knobIdx = s.knobIdxs[0]
			break
		}
	}
	if knobIdx < 0 {
		t.Fatalf("section %v has no knobs", secID)
	}
	knobR := g.drum.instEditorKnobs[knobIdx].Rect()
	// No viz band: the gap between the knob bottom and the cell bottom is just
	// the inter-row gap (SpaceXS), never a reserved drawing band.
	band := cell.Max.Y - knobR.Max.Y
	if band > SpaceXS {
		t.Fatalf("knob cell reserves %d px below the knob (cell=%v knob=%v) — Task 8 removed the per-knob viz band; expected <= %d (gap only)", band, cell, knobR, SpaceXS)
	}
}

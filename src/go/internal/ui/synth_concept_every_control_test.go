//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

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
	g.drum.setSelectedSynthSection(roster.Modular, secID)
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
	knobR := g.drum.instEditorControls[knobIdx].Rect()
	// No viz band: the gap between the knob bottom and the cell bottom is just
	// the inter-row gap (SpaceXS), never a reserved drawing band.
	band := cell.Max.Y - knobR.Max.Y
	if band > SpaceXS {
		t.Fatalf("knob cell reserves %d px below the knob (cell=%v knob=%v) — Task 8 removed the per-knob viz band; expected <= %d (gap only)", band, cell, knobR, SpaceXS)
	}
}

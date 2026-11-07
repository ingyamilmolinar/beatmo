//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// The cell reserves no band below the knob: the graph pane explains the focused dial.
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
	// Only the inter-row gap (SpaceXS).
	band := cell.Max.Y - knobR.Max.Y
	if band > SpaceXS {
		t.Fatalf("knob cell reserves %d px below the knob (cell=%v knob=%v) — Task 8 removed the per-knob viz band; expected <= %d (gap only)", band, cell, knobR, SpaceXS)
	}
}

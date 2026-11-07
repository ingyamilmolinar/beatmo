package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// The panel claims its expanded height only on a Layout that sees TabSynth, and Ebiten skips Layout at an
// unchanged size, so the framebuffer shrinks then grows to force one.
func expandSynthPanelForTest(t *testing.T, g *Game) {
	t.Helper()
	g.drum.eqPanelZone.SetActiveTab(TabSynth)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())
	g.Layout(640, 480)
	g.Layout(1280, 720)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())
	layoutSynthTab(t, g)
}

// Opens the stage that owns the knob, so the knob gets a hit-testable rect.
func selectSectionForKnobIdx(t *testing.T, g *Game, instID instrument.ID, knobIdx int) {
	t.Helper()
	dv := g.drum
	owner := synthSectionID(-1)
	for _, s := range dv.instEditorSections {
		for _, k := range s.knobIdxs {
			if k == knobIdx {
				owner = s.id
				break
			}
		}
		if owner != -1 {
			break
		}
	}
	if owner == -1 {
		t.Fatalf("no section owns knob index %d (sections=%v)", knobIdx, dv.instEditorSections)
	}
	dv.setSelectedSynthSection(instID, owner)
	// Fine-tuning knobs sit behind More.
	dv.setSynthMoreOpen(dv.resolveSynthInstrument(instID), true)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())
	// An Advanced knob's grid index is not its position in knobIdxs: the essentials pad to a full row and a phantom
	// divider row follows. Scroll to that index so the rect exists.
	for _, s := range dv.instEditorSections {
		if s.id != owner {
			continue
		}
		ess, adv := dv.synthSectionTierSplit(s.knobIdxs)
		grid := dv.sectionGrid(owner)
		cols := grid.Cols()
		if cols < 1 {
			cols = 1
		}
		gridIdx := -1
		for p, k := range ess {
			if k == knobIdx {
				gridIdx = p
			}
		}
		if gridIdx < 0 {
			base := ((len(ess)+cols-1)/cols)*cols + cols // padded ess rows + divider row
			for p, k := range adv {
				if k == knobIdx {
					gridIdx = base + p
				}
			}
		}
		if gridIdx >= 0 {
			grid.ScrollToIndex(gridIdx)
			g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())
		}
		return
	}
}

// Fails if none matches.
func synthBindingIdxByName(t *testing.T, g *Game, name string) int {
	t.Helper()
	for i, b := range g.drum.SynthTabBindings() {
		if b.def.Name == name {
			return i
		}
	}
	t.Fatalf("no %q knob in synth bindings", name)
	return -1
}

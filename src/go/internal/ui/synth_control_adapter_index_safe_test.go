package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// expandSynthPanelForTest activates the Synth tab and forces the audio panel
// to its expanded synth height (>= 240 px) so the detail pane is several knob
// rows tall. The panel only claims that height during a DrumView Layout pass
// that observes TabSynth active; Ebiten's Layout no-ops when the size is
// unchanged, so we toggle the framebuffer size (shrink then grow) after the
// tab is active to force a genuine relayout.
func expandSynthPanelForTest(t *testing.T, g *Game) {
	t.Helper()
	g.drum.eqPanelZone.SetActiveTab(TabSynth)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())
	g.Layout(640, 480)
	g.Layout(1280, 720)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())
	layoutSynthTab(t, g)
}

// selectSectionForKnobIdx opens the synth detail stage that owns the given
// knob index (so the knob gets a non-empty, hit-testable rect under the new
// chip-strip + expand-one-detail-pane layout), then re-lays-out the panel.
// instID is the row's resolved instrument id (e.g. "snare", "modular").
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
	// Fine-tuning knobs sit behind the More expander: open it so every knob of
	// the stage can acquire a rect.
	dv.setSynthMoreOpen(dv.resolveSynthInstrument(instID), true)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())
	// Card-cell layout (Task 3): a stage's later knobs can land on a scrolled-off
	// row. Compute the target knob's GRID index — which is NOT its position in
	// s.knobIdxs for Advanced knobs: the grid pads the essential tier up to a full
	// row, inserts a full phantom divider row, then lays out the advanced tier.
	// Scroll to that grid index and re-layout so its rect is populated.
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

// synthBindingIdxByName returns the binding index of the first knob whose
// param name matches, or fails the test.
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

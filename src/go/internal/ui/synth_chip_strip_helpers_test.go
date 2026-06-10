package ui

// Shared helpers for the synth-tab chip-strip tests. Deliberately UNTAGGED
// (no //go:build test) because both the fast-path (-tags test) suite and the
// untagged real-Ebiten test files (synth_modular_toggle_test.go,
// synth_disabled_stage_interactivity_test.go, …) use them.

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// layoutSynthTab switches the EQ panel to TabSynth and forces a layout pass.
func layoutSynthTab(t *testing.T, g *Game) {
	t.Helper()
	g.drum.eqPanelZone.SetActiveTab(TabSynth)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())
}

// newModularSynthTabGame is newSynthTabGame with the row bound to the modular
// voice (synth-modular prunes VOICE, so its first knobbed section is OSC).
func newModularSynthTabGame(t *testing.T) *Game {
	t.Helper()
	g := newSynthTabGame(t)
	g.drum.Rows[0].Instrument = "ut-chip-modular"
	audio.BindInstrumentToRecipe("ut-chip-modular", "synth-modular")
	t.Cleanup(func() { audio.ResetInstrumentParams("ut-chip-modular") })
	return g
}

// chipByID returns the chip for a section id, failing the test when absent.
func chipByID(t *testing.T, dv *DrumView, id synthSectionID) synthChip {
	t.Helper()
	for _, c := range dv.instEditorChips {
		if c.id == id {
			return c
		}
	}
	t.Fatalf("no chip for section %v (chips=%v)", id, dv.instEditorChips)
	return synthChip{}
}

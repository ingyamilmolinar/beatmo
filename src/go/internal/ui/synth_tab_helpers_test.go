package ui

// Shared helpers for the synth-tab tests. Deliberately UNTAGGED
// (no //go:build test) because both the fast-path (-tags test) suite and the
// untagged real-Ebiten test files (synth_modular_toggle_test.go,
// synth_disabled_stage_interactivity_test.go, …) use them.

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// layoutSynthTab switches the EQ panel to TabSynth and forces a layout pass.
// When the tab is at its ROOT (category cards + pipeline strip) it drills into
// the default stage — the first knobbed section whose stage is on — so the
// stage-semantics tests find laid-out controls, exactly as the pre-category
// layout opened its default stage. Root-specific tests use their own helpers.
func layoutSynthTab(t *testing.T, g *Game) {
	t.Helper()
	dv := g.drum
	dv.eqPanelZone.SetActiveTab(TabSynth)
	dv.eqPanelZone.Layout(dv.eqPanelZone.PanelRect())
	inst := dv.resolveSynthInstrument(dv.synthTabActiveInstrument())
	if inst == "" || !dv.synthAtMacroRoot(inst) {
		return
	}
	if id, ok := defaultStageFor(dv, string(inst)); ok {
		dv.setSynthStageOpen(inst, id)
		dv.eqPanelZone.Layout(dv.eqPanelZone.PanelRect())
	}
}

// defaultStageFor picks the stage a fresh drill lands on: the first section in
// pipeline order that has knobs AND is enabled; else the first knobbed section.
func defaultStageFor(dv *DrumView, inst string) (synthSectionID, bool) {
	fallback := synthSectionID(-1)
	for i := range dv.instEditorSections {
		s := &dv.instEditorSections[i]
		if len(s.knobIdxs) == 0 {
			continue
		}
		if fallback == -1 {
			fallback = s.id
		}
		if s.enableParam == "" || synthStageEnabled(instrument.ID(inst), s.enableParam) {
			return s.id, true
		}
	}
	if fallback != -1 {
		return fallback, true
	}
	return -1, false
}

// newModularSynthTabGame is newSynthTabGame with the row bound to the modular
// voice (synth-modular prunes VOICE, so its first knobbed section is OSC).
func newModularSynthTabGame(t *testing.T) *Game {
	t.Helper()
	g := newSynthTabGame(t)
	g.drum.Rows[0].Instrument = "ut-chip-modular"
	_ = audio.InstallInstrumentDocForTest("ut-chip-modular", roster.Modular)
	t.Cleanup(func() { audio.ResetInstrumentParams("ut-chip-modular") })
	return g
}

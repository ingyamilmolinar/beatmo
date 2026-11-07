package ui

// Untagged: both the -tags test and the real-build synth tests use them.

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// At the root it drills into the first knobbed, enabled stage, so stage tests find laid-out controls.
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

// The first knobbed, enabled section in pipeline order, else the first knobbed one.
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

// The modular voice prunes VOICE, so its first knobbed section is OSC.
func newModularSynthTabGame(t *testing.T) *Game {
	t.Helper()
	g := newSynthTabGame(t)
	g.drum.Rows[0].Instrument = "ut-chip-modular"
	_ = audio.InstallInstrumentDocForTest("ut-chip-modular", roster.Modular)
	t.Cleanup(func() { audio.ResetInstrumentParams("ut-chip-modular") })
	return g
}

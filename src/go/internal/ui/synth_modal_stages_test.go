//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// MODAL + STRIKE + NOISE on row 0.
func modalSynthGame(t *testing.T) (*Game, string) {
	t.Helper()
	const inst = roster.TubularBells
	g := newSynthTabGame(t)
	g.drum.Rows[0].Instrument = inst
	if err := installDemoModalDoc(inst); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = audio.InstallInstrumentDocForTest(inst, inst)
		audio.ResetInstrumentParams(inst)
	})
	g.drum.eqPanelZone.SetActiveTab(TabSynth)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())
	return g, string(inst)
}

func TestModalStagesShowOnlyOnADocumentWithRows(t *testing.T) {
	_, inst := modalSynthGame(t)
	order := synthSectionOrderForSchema(audio.InstrumentParamDefs(instrument.ID(inst)))
	pos := map[synthSectionID]int{}
	for i, s := range order {
		pos[s] = i + 1
	}
	if pos[synthSectionModal] == 0 || pos[synthSectionStrike] == 0 || pos[synthSectionNoise] == 0 {
		t.Fatalf("clone document lacks a MODAL / STRIKE / NOISE section: %v", order)
	}
	if !(pos[synthSectionModal] < pos[synthSectionStrike] && pos[synthSectionStrike] < pos[synthSectionNoise]) {
		t.Fatalf("order %v: want MODAL · STRIKE · NOISE", order)
	}
	// No table rows, so no stage, not even under "+ Add": knobs over an empty table would be silence.
	_ = audio.InstallInstrumentDocForTest(instrument.ID(inst), instrument.ID(inst))
	for _, id := range roster.All {
		for _, s := range synthSectionOrderForSchema(audio.InstrumentParamDefs(id)) {
			if s == synthSectionModal || s == synthSectionStrike || s == synthSectionNoise {
				t.Fatalf("%s shows %s without table rows", id, sectionLabel(s))
			}
		}
	}
}

func TestModalStagesAreOnInThePipelineAndTheirPillsToggle(t *testing.T) {
	g, inst := modalSynthGame(t)
	p := g.drum.synp()
	on, _ := p.activePipelineStages(instrument.ID(inst))
	onIDs := map[synthSectionID]bool{}
	for _, i := range on {
		onIDs[g.drum.instEditorSections[i].id] = true
	}
	for _, id := range []synthSectionID{synthSectionModal, synthSectionStrike, synthSectionNoise} {
		if !onIDs[id] {
			t.Fatalf("%s is not ON in the pipeline strip (on: %v)", sectionLabel(id), onIDs)
		}
	}
	for id, param := range map[synthSectionID]string{synthSectionStrike: "strike_enabled", synthSectionNoise: "noiseband_enabled"} {
		p.applySynthStageToggleByID(instrument.ID(inst), id)
		if v := audio.MergedInstrumentParamsRO(instrument.ID(inst))[param]; v != 0 {
			t.Fatalf("%s pill: %s = %v after the toggle, want 0", sectionLabel(id), param, v)
		}
		p.applySynthStageToggleByID(instrument.ID(inst), id)
		if v := audio.MergedInstrumentParamsRO(instrument.ID(inst))[param]; v != 1 {
			t.Fatalf("%s pill: %s = %v after the second toggle, want 1", sectionLabel(id), param, v)
		}
	}
}

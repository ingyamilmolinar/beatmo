package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// An empty VOICE is pruned; a populated VOICE shows a locked toggle (the voice is the instrument); ENSEMBLE and
// RESONATOR, whose off is a value, get interactive toggles that drive it; every shown stage has a pill.

func setupSynthGameForInst(t *testing.T, instID instrument.ID) *Game {
	t.Helper()
	g := newSynthTabGame(t)
	g.drum.Rows[0].Instrument = instID
	_ = audio.InstallInstrumentDocForTest(instID, instID)
	t.Cleanup(func() { audio.ResetInstrumentParams(instID) })
	g.drum.eqPanelZone.SetActiveTab(TabSynth)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())
	return g
}

// Its sound is the OSC stage, so there is nothing to do in VOICE.
func TestSynthTab_EmptyVoicePruned(t *testing.T) {
	g := setupModularSynthGame(t)
	layoutSynthTab(t, g)
	if _, ok := synthSectionByID(g.drum, synthSectionVoice); ok {
		t.Error("empty VOICE stage must be pruned on the modular voice (no visible knobs)")
	}
}

// Greyed and inert: the voice cannot be disabled.
func TestSynthTab_PopulatedVoiceLockedToggle(t *testing.T) {
	g := setupSynthGameForInst(t, roster.Violin)
	layoutSynthTab(t, g)

	sec, ok := synthSectionByID(g.drum, synthSectionVoice)
	if !ok {
		t.Fatal("violin VOICE stage (family knobs) must be shown")
	}
	if len(sec.KnobIdxs()) == 0 {
		t.Fatal("precondition: violin VOICE must carry family knobs")
	}

	selectStageProdHeight(t, g, roster.Violin, synthSectionVoice)

	// The locked pill is visible...
	if g.drum.synthDetailEnablePillRect().Empty() {
		t.Error("populated VOICE must show its (locked) enable pill")
	}
	// ...but non-interactive: no synth-toggle hit area is published for it.
	for _, a := range g.drum.synthTabHitAreas() {
		if a.Tag == "synth-toggle-voice" || a.Tag == "synth-toggle-" {
			t.Errorf("mandatory VOICE must not publish a toggle hit area (got %q)", a.Tag)
		}
	}
}

// unison_voices both ways (off = 1, on ≥ 2), with an interactive hit area.
func TestSynthTab_EnsembleToggleDrivesVoices(t *testing.T) {
	g := setupModularSynthGame(t)
	restore := SwapSynthAuditionFnForTest(func(instrument.ID) {})
	t.Cleanup(func() { SwapSynthAuditionFnForTest(restore) })

	// Start from a known "on" state so the first tap turns it off.
	audio.SetInstrumentParam(roster.Modular, "unison_voices", 4)

	selectStageProdHeight(t, g, roster.Modular, synthSectionEnsemble)
	pill := g.drum.synthDetailEnablePillRect()
	if pill.Empty() {
		t.Fatal("ENSEMBLE must show an enable pill")
	}
	// Interactive → publishes a toggle hit area.
	seen := false
	for _, a := range g.drum.synthTabHitAreas() {
		if a.Tag == "synth-toggle-unison_voices" {
			seen = true
		}
	}
	if !seen {
		t.Error("interactive ENSEMBLE pill must publish its synth-toggle hit area")
	}

	voices := func() float64 { return audio.MergedInstrumentParamsRO(roster.Modular)["unison_voices"] }
	cx, cy := (pill.Min.X+pill.Max.X)/2, (pill.Min.Y+pill.Max.Y)/2

	// Tap 1: on (4) → off (1).
	g.drum.handleSynthTabInput(cx, cy, true, roster.Modular)
	g.drum.handleSynthTabInput(cx, cy, false, roster.Modular)
	if voices() >= 2 {
		t.Errorf("tapping ENSEMBLE while on must set unison_voices to 1 (got %v)", voices())
	}
	// Tap 2: off (1) → on (>= 2).
	g.drum.handleSynthTabInput(cx, cy, true, roster.Modular)
	g.drum.handleSynthTabInput(cx, cy, false, roster.Modular)
	if voices() < 2 {
		t.Errorf("tapping ENSEMBLE while off must set unison_voices to an active default >= 2 (got %v)", voices())
	}
}

// body_model both ways (off = 0, on ≥ 1).
func TestSynthTab_ResonatorToggleDrivesBodyModel(t *testing.T) {
	g := setupModularSynthGame(t)
	restore := SwapSynthAuditionFnForTest(func(instrument.ID) {})
	t.Cleanup(func() { SwapSynthAuditionFnForTest(restore) })

	audio.SetInstrumentParam(roster.Modular, "body_model", 2)

	selectStageProdHeight(t, g, roster.Modular, synthSectionResonator)
	pill := g.drum.synthDetailEnablePillRect()
	if pill.Empty() {
		t.Fatal("RESONATOR must show an enable pill")
	}
	model := func() float64 { return audio.MergedInstrumentParamsRO(roster.Modular)["body_model"] }
	cx, cy := (pill.Min.X+pill.Max.X)/2, (pill.Min.Y+pill.Max.Y)/2

	g.drum.handleSynthTabInput(cx, cy, true, roster.Modular)
	g.drum.handleSynthTabInput(cx, cy, false, roster.Modular)
	if model() >= 1 {
		t.Errorf("tapping RESONATOR while on must set body_model to Off (got %v)", model())
	}
	g.drum.handleSynthTabInput(cx, cy, true, roster.Modular)
	g.drum.handleSynthTabInput(cx, cy, false, roster.Modular)
	if model() < 1 {
		t.Errorf("tapping RESONATOR while off must set body_model to an active default >= 1 (got %v)", model())
	}
}

// Interactive or locked; no shown stage lacks one.
func TestSynthTab_EveryShownStageHasPill(t *testing.T) {
	g := setupModularSynthGame(t)
	for _, s := range g.drum.SynthTabSections() {
		selectStageProdHeight(t, g, roster.Modular, s.SectionID())
		if g.drum.synthDetailEnablePillRect().Empty() {
			t.Errorf("shown stage %q has no enable pill", s.Label())
		}
	}
}

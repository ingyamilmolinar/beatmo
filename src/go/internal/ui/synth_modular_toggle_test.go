package ui

import (
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// False when the recipe pruned it.
func synthSectionByID(dv *DrumView, id synthSectionID) (synthSection, bool) {
	for _, s := range dv.SynthTabSections() {
		if s.SectionID() == id {
			return s, true
		}
	}
	return synthSection{}, false
}

// At production height: the 640×480 harness leaves an ~80 px panel whose header drops the pill. Activate, resize to
// 1280×720, re-activate, select, re-activate.
func selectStageProdHeight(t *testing.T, g *Game, instID instrument.ID, id synthSectionID) {
	t.Helper()
	layoutSynthTab(t, g)
	g.Layout(1280, 720)
	layoutSynthTab(t, g)
	g.drum.setSelectedSynthSection(instID, id)
	layoutSynthTab(t, g)
}

// Every stage owns its enable param and shows its pill in the detail header when selected; VOICE never has one (it
// IS the instrument). POST's toggle is drive_enabled everywhere.
func TestSynthTab_ModularSectionsHaveEnablePills(t *testing.T) {
	g := setupModularSynthGame(t)
	want := map[synthSectionID]string{
		synthSectionOsc:       "osc_enabled",
		synthSectionFM:        "fm_enabled",
		synthSectionPitch:     "pitchenv_enabled",
		synthSectionLFO:       "lfo_enabled",
		synthSectionBurst:     "burst_enabled",
		synthSectionEnvelope:  "env_enabled",
		synthSectionFilter:    "filter_enabled",
		synthSectionFilterEnv: "filtenv_enabled",
		synthSectionPost:      "drive_enabled",
	}
	seen := map[synthSectionID]bool{}
	for _, s := range g.drum.SynthTabSections() {
		seen[s.SectionID()] = true
		if wantParam, isStage := want[s.SectionID()]; isStage && s.EnableParam() != wantParam {
			t.Errorf("section %q enableParam = %q, want %q", s.Label(), s.EnableParam(), wantParam)
		}
	}
	for id, param := range want {
		if !seen[id] {
			t.Errorf("modular tab missing section for enable param %q", param)
			continue
		}
		// Non-empty and inside the detail pane.
		selectStageProdHeight(t, g, roster.Modular, id)
		pill := g.drum.synthDetailEnablePillRect()
		if pill.Empty() {
			t.Errorf("selected stage %q (%s) has no detail enable pill", sectionLabel(id), param)
			continue
		}
		if !pill.In(g.drum.instEditorMacroDetailR) {
			t.Errorf("section %q pill %v not inside detail pane %v", sectionLabel(id), pill, g.drum.instEditorMacroDetailR)
		}
	}
	// VOICE never shows a pill (synth-modular prunes VOICE, but guard anyway).
	if _, ok := synthSectionByID(g.drum, synthSectionVoice); ok {
		selectStageProdHeight(t, g, roster.Modular, synthSectionVoice)
		if !g.drum.synthDetailEnablePillRect().Empty() {
			t.Errorf("VOICE section must not show an enable pill")
		}
	}

	// The modular snare's stages carry toggles too; its VOICE none.
	g2 := newSynthTabGame(t)
	layoutSynthTab(t, g2)
	drumWant := map[synthSectionID]string{
		synthSectionOsc:       "osc_enabled",
		synthSectionFM:        "fm_enabled",
		synthSectionPitch:     "pitchenv_enabled",
		synthSectionLFO:       "lfo_enabled",
		synthSectionBurst:     "burst_enabled",
		synthSectionEnvelope:  "env_enabled",
		synthSectionFilter:    "filter_enabled",
		synthSectionFilterEnv: "filtenv_enabled",
		synthSectionPost:      "drive_enabled",
	}
	for _, s := range g2.drum.SynthTabSections() {
		if s.SectionID() == synthSectionVoice {
			if s.EnableParam() != "" {
				t.Errorf("drum VOICE section unexpectedly has enable param %q", s.EnableParam())
			}
			continue
		}
		if wantParam, isStage := drumWant[s.SectionID()]; isStage && s.EnableParam() != wantParam {
			t.Errorf("drum recipe section %q enableParam = %q, want %q", s.Label(), s.EnableParam(), wantParam)
		}
	}
	for id := range drumWant {
		if _, ok := synthSectionByID(g2.drum, id); !ok {
			continue
		}
		selectStageProdHeight(t, g2, roster.Snare, id)
		if g2.drum.synthDetailEnablePillRect().Empty() {
			t.Errorf("drum recipe selected stage %q has no detail enable pill", sectionLabel(id))
		}
	}
}

// Engine-internal: in the ABI, never an editable knob.
func TestSynthTab_NoiseSeedNotAGridControl(t *testing.T) {
	g := setupModularSynthGame(t)
	for _, s := range g.drum.SynthTabSections() {
		for _, idx := range s.KnobIdxs() {
			b := g.drum.SynthTabBindings()[idx]
			if b.def.Name == "noise_seed" {
				t.Errorf("noise_seed is rendered as a knob in section %q (must be hidden)", s.Label())
			}
			if isSynthEnableParam(b.def.Name) {
				t.Errorf("enable param %q is rendered as a knob in section %q (must be a pill)", b.def.Name, s.Label())
			}
		}
	}
}

// FILTER's pill flips filter_enabled 0/1 through the knob plumbing, starting enabled so the sound is unchanged. OSC,
// the sole source, is greyed instead (synth_essential_stage_test.go).
func TestSynthTab_EnablePillTogglesStage(t *testing.T) {
	g := setupModularSynthGame(t)
	// Silence the audition so the test makes no sound.
	restore := SwapSynthAuditionFnForTest(func(instrument.ID) {})
	t.Cleanup(func() { SwapSynthAuditionFnForTest(restore) })

	if !synthStageEnabled(roster.Modular, "filter_enabled") {
		t.Fatal("filter stage should start enabled (default 1)")
	}
	selectStageProdHeight(t, g, roster.Modular, synthSectionFilter)
	pill := g.drum.synthDetailEnablePillRect()
	if pill.Empty() {
		t.Fatal("filter enable pill rect is empty")
	}
	cx, cy := (pill.Min.X+pill.Max.X)/2, (pill.Min.Y+pill.Max.Y)/2

	if !g.drum.handleSynthTabInput(cx, cy, true, roster.Modular) {
		t.Fatal("tap on filter pill was not handled")
	}
	if synthStageEnabled(roster.Modular, "filter_enabled") {
		t.Errorf("filter stage still enabled after first tap (want bypassed)")
	}
	if g.drum.handleSynthTabInput(cx, cy, true, roster.Modular); synthStageEnabled(roster.Modular, "filter_enabled") == false {
		t.Errorf("filter stage still bypassed after second tap (want re-enabled)")
	}
}

// Each selected stage publishes exactly its own synth-toggle-<param>; unselected stages publish none.
func TestSynthTab_EnablePillHasHitArea(t *testing.T) {
	g := setupModularSynthGame(t)
	// OSC is absent: as the sole source it is greyed and publishes none.
	stages := map[synthSectionID]string{
		synthSectionFM:        "synth-toggle-fm_enabled",
		synthSectionPitch:     "synth-toggle-pitchenv_enabled",
		synthSectionLFO:       "synth-toggle-lfo_enabled",
		synthSectionBurst:     "synth-toggle-burst_enabled",
		synthSectionEnvelope:  "synth-toggle-env_enabled",
		synthSectionFilter:    "synth-toggle-filter_enabled",
		synthSectionFilterEnv: "synth-toggle-filtenv_enabled",
		synthSectionPost:      "synth-toggle-drive_enabled",
	}
	for id, wantTag := range stages {
		selectStageProdHeight(t, g, roster.Modular, id)
		seen := map[string]bool{}
		for _, a := range g.drum.synthTabHitAreas() {
			if strings.HasPrefix(a.Tag, "synth-toggle-") {
				seen[a.Tag] = true
			}
		}
		if !seen[wantTag] {
			t.Errorf("selected stage %q missing pill hit area %q", sectionLabel(id), wantTag)
		}
		for otherID, otherTag := range stages {
			if otherID == id {
				continue
			}
			if seen[otherTag] {
				t.Errorf("unselected stage published pill hit area %q while %q is selected", otherTag, sectionLabel(id))
			}
		}
	}

	selectStageProdHeight(t, g, roster.Modular, synthSectionOsc)
	for _, a := range g.drum.synthTabHitAreas() {
		if a.Tag == "synth-toggle-osc_enabled" {
			t.Error("essential OSC stage must not publish a synth-toggle hit area")
		}
	}
}

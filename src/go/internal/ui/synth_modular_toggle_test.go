package ui

import (
	"strings"
	"testing"
)

// synthSectionByID returns the live section for id from the current layout, or
// false when the recipe pruned it.
func synthSectionByID(dv *DrumView, id synthSectionID) (synthSection, bool) {
	for _, s := range dv.SynthTabSections() {
		if s.SectionID() == id {
			return s, true
		}
	}
	return synthSection{}, false
}

// selectStageProdHeight opens id for instID with a production-height audio
// panel (the 640×480 harness leaves the panel only ~80 px, degenerating the
// detail header to ~16 px so the enable pill is legitimately dropped). The
// panel claims its expanded height on the Layout AFTER the tab activates, so
// the dance is: activate, resize to 1280×720, re-activate, then select +
// re-activate. Mirrors TestSynthDetailPane_EnablePillForSelectedStage.
func selectStageProdHeight(t *testing.T, g *Game, instID string, id synthSectionID) {
	t.Helper()
	layoutSynthTab(t, g)
	g.Layout(1280, 720)
	layoutSynthTab(t, g)
	g.drum.setSelectedSynthSection(instID, id)
	layoutSynthTab(t, g)
}

// TestSynthTab_ModularSectionsHaveEnablePills — every standardized pipeline
// stage owns the right enable param, and (in the redesign) surfaces its enable
// pill in the detail-pane header when that stage is the SELECTED one. The
// per-card pill is gone; the single pill lives in the detail header of the
// open stage. The VOICE section (family knobs) never grows a pill — the voice
// IS the instrument. This holds for the modular voice (POST toggle is
// drive_enabled) AND every migrated drum/FM instrument (POST toggle is
// post_enabled) — the unification deliverable.
func TestSynthTab_ModularSectionsHaveEnablePills(t *testing.T) {
	g := setupModularSynthGame(t)
	want := map[synthSectionID]string{
		synthSectionOsc:      "osc_enabled",
		synthSectionFM:       "fm_enabled",
		synthSectionPitch:    "pitchenv_enabled",
		synthSectionLFO:      "lfo_enabled",
		synthSectionBurst:    "burst_enabled",
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
		// Select the stage at production height and assert its detail-header
		// pill is non-empty and inside the detail pane.
		selectStageProdHeight(t, g, "modular", id)
		pill := g.drum.synthDetailEnablePillRect()
		if pill.Empty() {
			t.Errorf("selected stage %q (%s) has no detail enable pill", sectionLabel(id), param)
			continue
		}
		if !pill.In(g.drum.instEditorDetailR) {
			t.Errorf("section %q pill %v not inside detail pane %v", sectionLabel(id), pill, g.drum.instEditorDetailR)
		}
	}
	// VOICE never shows a pill (synth-modular prunes VOICE, but guard anyway).
	if _, ok := synthSectionByID(g.drum, synthSectionVoice); ok {
		selectStageProdHeight(t, g, "modular", synthSectionVoice)
		if !g.drum.synthDetailEnablePillRect().Empty() {
			t.Errorf("VOICE section must not show an enable pill")
		}
	}

	// A migrated drum recipe (drum-snare) now ALSO carries per-stage enable
	// toggles on its standardized stage sections (the unification deliverable),
	// each surfaced via the detail-header pill when selected. The VOICE section
	// (family knobs) carries none.
	g2 := newSynthTabGame(t)
	layoutSynthTab(t, g2)
	drumWant := map[synthSectionID]string{
		synthSectionOsc:      "osc_enabled",
		synthSectionFM:       "fm_enabled",
		synthSectionPitch:    "pitchenv_enabled",
		synthSectionLFO:      "lfo_enabled",
		synthSectionBurst:    "burst_enabled",
		synthSectionEnvelope:  "env_enabled",
		synthSectionFilter:    "filter_enabled",
		synthSectionFilterEnv: "filtenv_enabled",
		synthSectionPost:      "post_enabled",
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
		selectStageProdHeight(t, g2, "snare", id)
		if g2.drum.synthDetailEnablePillRect().Empty() {
			t.Errorf("drum recipe selected stage %q has no detail enable pill", sectionLabel(id))
		}
	}
}

// TestSynthTab_NoiseSeedNotAGridKnob — the engine-internal noise_seed param is
// present in the schema/ABI but must not appear as an editable knob.
func TestSynthTab_NoiseSeedNotAGridKnob(t *testing.T) {
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

// TestSynthTab_EnablePillTogglesStage — tapping the OSC enable pill flips
// osc_enabled to 0 (bypassed) and back to 1 (enabled), through the same
// SetInstrumentParam plumbing a knob uses. Defaults must start enabled so the
// sound is unchanged until the user toggles.
func TestSynthTab_EnablePillTogglesStage(t *testing.T) {
	g := setupModularSynthGame(t)
	// Silence the audition so the test makes no sound.
	restore := SwapSynthAuditionFnForTest(func(string) {})
	t.Cleanup(func() { SwapSynthAuditionFnForTest(restore) })

	if !synthStageEnabled("modular", "osc_enabled") {
		t.Fatal("osc stage should start enabled (default 1)")
	}
	// Select the OSC stage at production height so its detail-header pill is
	// laid out, then read it from the detail pane (the per-card pill is gone).
	selectStageProdHeight(t, g, "modular", synthSectionOsc)
	pill := g.drum.synthDetailEnablePillRect()
	if pill.Empty() {
		t.Fatal("osc enable pill rect is empty")
	}
	cx, cy := (pill.Min.X+pill.Max.X)/2, (pill.Min.Y+pill.Max.Y)/2

	if !g.drum.handleSynthTabInput(cx, cy, true, "modular") {
		t.Fatal("tap on osc pill was not handled")
	}
	if synthStageEnabled("modular", "osc_enabled") {
		t.Errorf("osc stage still enabled after first tap (want bypassed)")
	}
	if g.drum.handleSynthTabInput(cx, cy, true, "modular"); synthStageEnabled("modular", "osc_enabled") == false {
		t.Errorf("osc stage still bypassed after second tap (want re-enabled)")
	}
}

// TestSynthTab_EnablePillHasHitArea — the detail-header pill publishes a tagged
// HitArea for the SELECTED stage only (parity with the direct-input path); a
// stage that is not selected publishes none (its pill lives only in the open
// detail pane). Each stage, when selected at production height, must publish
// exactly its own synth-toggle-<param> and no other.
func TestSynthTab_EnablePillHasHitArea(t *testing.T) {
	g := setupModularSynthGame(t)
	stages := map[synthSectionID]string{
		synthSectionOsc:      "synth-toggle-osc_enabled",
		synthSectionFM:       "synth-toggle-fm_enabled",
		synthSectionPitch:    "synth-toggle-pitchenv_enabled",
		synthSectionLFO:      "synth-toggle-lfo_enabled",
		synthSectionBurst:    "synth-toggle-burst_enabled",
		synthSectionEnvelope:  "synth-toggle-env_enabled",
		synthSectionFilter:    "synth-toggle-filter_enabled",
		synthSectionFilterEnv: "synth-toggle-filtenv_enabled",
		synthSectionPost:      "synth-toggle-drive_enabled",
	}
	for id, wantTag := range stages {
		selectStageProdHeight(t, g, "modular", id)
		seen := map[string]bool{}
		for _, a := range g.drum.synthTabHitAreas() {
			if strings.HasPrefix(a.Tag, "synth-toggle-") {
				seen[a.Tag] = true
			}
		}
		if !seen[wantTag] {
			t.Errorf("selected stage %q missing pill hit area %q", sectionLabel(id), wantTag)
		}
		// No OTHER stage's pill hit area should be published while it is
		// unselected.
		for otherID, otherTag := range stages {
			if otherID == id {
				continue
			}
			if seen[otherTag] {
				t.Errorf("unselected stage published pill hit area %q while %q is selected", otherTag, sectionLabel(id))
			}
		}
	}
}

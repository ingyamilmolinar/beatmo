//go:build test

package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Every shipped synth instrument lays out a subsequence of the standard order (VOICE · OSC · ENSEMBLE · FM · PITCH ·
// LFO · BURST · ENVELOPE · FILTER · FILTER ENV · FORMANT · RESONATOR · POST): sections may be pruned but never
// reordered, and POST is always present.
func TestSynthUnified_EverySynthInstrumentUsesStandardOrder(t *testing.T) {
	rank := map[synthSectionID]int{
		synthSectionVoice:     0,
		synthSectionOsc:       1,
		synthSectionKick:      2,
		synthSectionEnsemble:  3,
		synthSectionFM:        4,
		synthSectionPitch:     5,
		synthSectionLFO:       6,
		synthSectionBurst:     7,
		synthSectionEnvelope:  8,
		synthSectionFilter:    9,
		synthSectionFilterEnv: 10,
		synthSectionFormant:   11,
		synthSectionResonator: 12,
		synthSectionPost:      13,
		synthSectionFinish:    14,
	}
	for _, id := range roster.All {
		order := synthSectionOrderForSchema(audio.InstrumentParamDefs(id))
		if len(order) == 0 {
			t.Errorf("instrument %q: no sections (every synth instrument must show at least POST)", id)
			continue
		}
		last := -1
		havePost := false
		for _, sec := range order {
			r, ok := rank[sec]
			if !ok {
				t.Errorf("instrument %q: section %v is not part of the standardized order", id, sec)
				continue
			}
			if r <= last {
				t.Errorf("instrument %q: section order %v is not the standardized subsequence (VOICE·OSC·ENSEMBLE·FM·PITCH·LFO·BURST·ENVELOPE·FILTER·FILTER ENV·FORMANT·RESONATOR·POST)", id, order)
				break
			}
			last = r
			if sec == synthSectionPost {
				havePost = true
			}
		}
		if !havePost {
			t.Errorf("instrument %q: standardized order %v is missing the POST section", id, order)
		}
	}
}

// The formant, unison and resonator groups route to their sections, and the FORMANT toggle owns its section.
func TestVoiceStagesRouting(t *testing.T) {
	if sec, ok := modularStageGroupSection("unison"); !ok || sec != synthSectionEnsemble {
		t.Fatalf("unison group must route to ENSEMBLE, got %v ok=%v", sec, ok)
	}
	if sec, ok := modularStageGroupSection("formant"); !ok || sec != synthSectionFormant {
		t.Fatalf("formant group must route to FORMANT, got %v ok=%v", sec, ok)
	}
	if sec, ok := modularStageGroupSection("resonator"); !ok || sec != synthSectionResonator {
		t.Fatalf("resonator group must route to RESONATOR, got %v ok=%v", sec, ok)
	}
	if sec, ok := enableToggleSection("formant_enabled"); !ok || sec != synthSectionFormant {
		t.Fatalf("formant_enabled pill must own FORMANT, got %v ok=%v", sec, ok)
	}
}

// Each stage an instrument has stays in the strip model (off under "+") and shows its enable pill when selected at
// production height; VOICE never has one, and a stage the instrument declares no toggle for needs none.
func TestSynthUnified_EveryStageSectionHasEnablePill(t *testing.T) {
	// section → the *_enabled toggle that gates it.
	stageToggle := map[synthSectionID][]string{
		synthSectionOsc:       {"osc_enabled"},
		synthSectionKick:      {"kick_enabled"},
		synthSectionFM:        {"fm_enabled"},
		synthSectionPitch:     {"pitchenv_enabled"},
		synthSectionLFO:       {"lfo_enabled"},
		synthSectionBurst:     {"burst_enabled"},
		synthSectionEnvelope:  {"env_enabled"},
		synthSectionFilter:    {"filter_enabled"},
		synthSectionFilterEnv: {"filtenv_enabled"},
		synthSectionPost:      {"drive_enabled"},
	}
	// On or off: an off stage lives under "+".
	stageInPipeline := func(dv *DrumView, id synthSectionID) bool {
		on, off := dv.synp().activePipelineStages(dv.synthTabActiveInstrument())
		for _, i := range append(on, off...) {
			if dv.instEditorSections[i].id == id {
				return true
			}
		}
		return false
	}
	for _, libID := range roster.All {
		instID := "ut-" + libID
		if err := audio.InstallInstrumentDocForTest(instID, libID); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { audio.RemoveInstrumentDocForTest(instID) })
		g := newSynthTabGame(t)
		g.drum.Rows[0].Instrument = instID
		// The 640×480 harness degenerates the detail header.
		layoutSynthTab(t, g)
		g.Layout(1280, 720)
		layoutSynthTab(t, g)

		// VOICE never carries an enable pill.
		for _, s := range g.drum.SynthTabSections() {
			if s.SectionID() == synthSectionVoice && s.EnableParam() != "" {
				t.Errorf("instrument %q: VOICE section has enable pill %q (the voice has none)", libID, s.EnableParam())
			}
		}

		sawStageSection := false
		for _, s := range g.drum.SynthTabSections() {
			toggles, isStage := stageToggle[s.SectionID()]
			if !isStage {
				continue
			}
			// A collision-excluded stage is never shown, and a recipe wiring only generic knobs into ENVELOPE/POST has no toggle.
			ownsToggle := false
			for _, tg := range toggles {
				if instHasVisibleParam(string(libID), tg) {
					ownsToggle = true
				}
			}
			if !ownsToggle {
				continue
			}
			sawStageSection = true
			if s.EnableParam() == "" {
				t.Errorf("instrument %q: stage section %q has no enable param", libID, s.Label())
				continue
			}
			// (a) the stage is part of the pipeline model (strip pill or "+").
			if !stageInPipeline(g.drum, s.SectionID()) {
				t.Errorf("instrument %q: stage section %q is not in the pipeline strip model", libID, s.Label())
			}
			// (b) when selected, the detail-header pill is non-empty.
			g.drum.setSelectedSynthSection(instID, s.SectionID())
			layoutSynthTab(t, g)
			if g.drum.synthDetailEnablePillRect().Empty() {
				t.Errorf("instrument %q: selected stage section %q has no detail enable pill", libID, s.Label())
			}
		}
		if !sawStageSection {
			t.Errorf("instrument %q: shows no standardized stage section with an enable pill", libID)
		}
		audio.ResetInstrumentParams(instID)
	}
}

// The "not used by this sound" hint never returns: empty sections are pruned instead.
func TestSynthUnified_NoCollapsedHintInSource(t *testing.T) {
	const forbidden = "not used by this sound"
	root := "."
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		// Allow the discipline test itself to mention the literal.
		if strings.HasSuffix(path, "synth_unified_sections_test.go") {
			return nil
		}
		b, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		if strings.Contains(string(b), forbidden) {
			t.Errorf("forbidden collapsed-section hint string %q found in %s", forbidden, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk internal/ui: %v", err)
	}
}

// Non-hidden, so it would render as a knob or pill.
func instHasVisibleParam(id, name string) bool {
	for _, d := range audio.InstrumentParamDefs(instrument.ID(id)) {
		if d.Name == name {
			return d.Group != audio.SynthHiddenGroup
		}
	}
	return false
}

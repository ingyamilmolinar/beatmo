//go:build test

package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// TestSynthUnified_EverySynthRecipeUsesStandardOrder is the Phase-8B
// unification discipline: every shipped synth (non-WAV) recipe lays out as a
// PREFIX-preserving subsequence of the standardized pipeline order
//
//	VOICE · OSC · ENSEMBLE · FM · PITCH · LFO · BURST · ENVELOPE · FILTER ·
//	FILTER ENV · FORMANT · RESONATOR · POST
//
// (PITCH/LFO/BURST are the Phase-8C modulator stages — the spec-§1 gap
// closure; ENSEMBLE/FORMANT/RESONATOR are the Phase-15 voice/choir stages —
// Task 8.) Sections may be pruned (a recipe with no FM stage has no FM card),
// but the surviving sections must always appear in this exact relative order,
// and POST must always be present (every synth carries gain + a post toggle,
// or a generic post knob). This is the user-facing deliverable: the same
// sections, in the same order, for every synth instrument.
func TestSynthUnified_EverySynthRecipeUsesStandardOrder(t *testing.T) {
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
	}
	for id, reg := range audio.RecipeRegistrations() {
		if reg == nil {
			continue
		}
		order := synthSectionOrderForSchema(reg.Params)
		if len(order) == 0 {
			t.Errorf("recipe %q: no sections (every synth recipe must show at least POST)", id)
			continue
		}
		last := -1
		havePost := false
		for _, sec := range order {
			r, ok := rank[sec]
			if !ok {
				t.Errorf("recipe %q: section %v is not part of the standardized order", id, sec)
				continue
			}
			if r <= last {
				t.Errorf("recipe %q: section order %v is not the standardized subsequence (VOICE·OSC·ENSEMBLE·FM·PITCH·LFO·BURST·ENVELOPE·FILTER·FILTER ENV·FORMANT·RESONATOR·POST)", id, order)
				break
			}
			last = r
			if sec == synthSectionPost {
				havePost = true
			}
		}
		if !havePost {
			t.Errorf("recipe %q: standardized order %v is missing the POST section", id, order)
		}
	}
}

// TestVoiceStagesRouting is the Task 8 routing guard: the Phase-15 voice/choir
// param Groups (formant, unison, resonator — carried verbatim from
// ModularSynthParamDefs) route to their standardized ENSEMBLE/FORMANT/RESONATOR
// sections, and the FORMANT enable toggle owns the FORMANT section.
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

// TestSynthUnified_EveryStageSectionHasEnablePill — the "enable/disable any
// stage on any synth instrument" deliverable, in the chip-strip redesign: for
// every shipped synth recipe, each standardized STAGE section the recipe
// appended (OSC/FM/PITCH/LFO/BURST/ENVELOPE/FILTER + the POST stage)
//
//	(a) appears as a chip in dv.instEditorChips — a disabled stage stays
//	    visible as a ghost chip, so the full pipeline never disappears; and
//	(b) when SELECTED at production panel height, surfaces its enable pill in
//	    the detail-pane header (dv.synthDetailEnablePillRect() is non-empty).
//
// The VOICE section never carries an enable pill (the voice IS the instrument).
// The per-section expectation keys off whether the recipe actually appended
// that stage's enable toggle — a recipe whose voice IS the FM stage (the fm-*
// family) has the FM stage collision-excluded, so it shows no FM chip and is
// not required to carry an fm_enabled pill.
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
		synthSectionPost:      {"drive_enabled", "post_enabled"},
	}
	chipFor := func(dv *DrumView, id synthSectionID) (synthChip, bool) {
		for _, c := range dv.instEditorChips {
			if c.id == id {
				return c, true
			}
		}
		return synthChip{}, false
	}
	for _, recipeID := range synthRecipeIDsForTest(t) {
		instID := "ut-" + recipeID
		audio.BindInstrumentToRecipe(instID, recipeID)
		g := newSynthTabGame(t)
		g.drum.Rows[0].Instrument = instID
		// Production-like panel height so the detail-pane enable pill is laid
		// out (the 640×480 harness degenerates the detail header). Activate the
		// tab, resize, re-activate.
		layoutSynthTab(t, g)
		g.Layout(1280, 720)
		layoutSynthTab(t, g)

		// VOICE never carries an enable pill.
		for _, s := range g.drum.SynthTabSections() {
			if s.SectionID() == synthSectionVoice && s.EnableParam() != "" {
				t.Errorf("recipe %q: VOICE section has enable pill %q (the voice has none)", recipeID, s.EnableParam())
			}
		}

		sawStageSection := false
		for _, s := range g.drum.SynthTabSections() {
			toggles, isStage := stageToggle[s.SectionID()]
			if !isStage {
				continue
			}
			// Does the recipe actually own this stage's enable toggle in its
			// schema? (A collision-excluded stage section is never shown, and a
			// plugin recipe that only wires generic knobs into ENVELOPE/POST
			// carries no toggle there — legitimately no pill.)
			ownsToggle := false
			for _, tg := range toggles {
				if recipeHasVisibleParam(recipeID, tg) {
					ownsToggle = true
				}
			}
			if !ownsToggle {
				continue
			}
			sawStageSection = true
			if s.EnableParam() == "" {
				t.Errorf("recipe %q: stage section %q has no enable param", recipeID, s.Label())
				continue
			}
			// (a) the stage appears as a chip in the strip.
			if _, ok := chipFor(g.drum, s.SectionID()); !ok {
				t.Errorf("recipe %q: stage section %q is not a chip in the strip", recipeID, s.Label())
			}
			// (b) when selected, the detail-header pill is non-empty.
			g.drum.setSelectedSynthSection(instID, s.SectionID())
			layoutSynthTab(t, g)
			if g.drum.synthDetailEnablePillRect().Empty() {
				t.Errorf("recipe %q: selected stage section %q has no detail enable pill", recipeID, s.Label())
			}
		}
		if !sawStageSection {
			t.Errorf("recipe %q: shows no standardized stage section with an enable pill", recipeID)
		}
		audio.ResetInstrumentParams(instID)
	}
}

// TestSynthUnified_NoCollapsedHintInSource is the repo-wide discipline that the
// removed "not used by this sound" collapsed-section hint never reappears in
// the UI source. Phase 8B replaced it with always-populated stage cards + an
// enable pill, and prunes empty sections instead.
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

// recipeHasVisibleParam reports whether recipeID's schema carries a non-hidden
// param named name (so the Synth tab would render it as a knob or pill).
func recipeHasVisibleParam(recipeID, name string) bool {
	reg, ok := audio.RecipeRegistrations()[recipeID]
	if !ok || reg == nil {
		return false
	}
	for _, d := range reg.Params {
		if d.Name == name {
			return d.Group != audio.SynthHiddenGroup
		}
	}
	return false
}

// synthRecipeIDsForTest returns the SHIPPED synth recipe ids (the canonical
// builtin drum/fm/modular recipes in RecipeOrder) that produce a renderable
// Synth tab. Transient test-registered plugin recipes are excluded — they may
// legitimately wire only generic knobs with no standardized stage params (hence
// no enable pills), so they are not part of the unification contract.
func synthRecipeIDsForTest(t *testing.T) []string {
	t.Helper()
	regs := audio.RecipeRegistrations()
	var out []string
	for _, id := range audio.RecipeOrder() {
		reg := regs[id]
		if reg == nil || len(reg.Params) == 0 {
			continue
		}
		switch reg.Category {
		case "drum", "fm", "modular":
			// Only the SHIPPED migrated/modular recipes carry standardized stage
			// toggles; a transient test-registered plugin recipe (also Category
			// "drum") wires only generic knobs and is not part of the unification
			// contract — exclude it by requiring at least one visible stage toggle.
			if recipeHasVisibleParam(id, "osc_enabled") ||
				recipeHasVisibleParam(id, "env_enabled") ||
				recipeHasVisibleParam(id, "filter_enabled") ||
				recipeHasVisibleParam(id, "drive_enabled") ||
				recipeHasVisibleParam(id, "post_enabled") {
				out = append(out, id)
			}
		}
	}
	return out
}

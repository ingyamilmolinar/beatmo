package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Every Params entry in both concept tables must be a real engine ParamDef —
// guards against typos and param renames drifting the tables
// away from the engine schema.
func TestMacroGroups_NoParamNameTypos(t *testing.T) {
	real := map[string]bool{}
	for _, d := range audio.InstrumentParamDefs(roster.Modular) {
		real[d.Name] = true
	}
	for _, tbl := range [][]macroGroupDef{macroGroupDefsMelodic, macroGroupDefsPercussion} {
		for _, def := range tbl {
			for _, p := range def.Params {
				if !real[p] {
					t.Errorf("concept %q references unknown param %q", def.ID, p)
				}
			}
		}
	}
}

// The percussion Source concept must own the discrete engine pickers exactly once
// and never fuse them into a continuous macro (Global Constraint).
func TestMacroGroups_PercussionPickersInOneConcept(t *testing.T) {
	pickers := []string{"osc_type", "fm_algorithm", "filter_type", "body_model"}
	owner := map[string]string{}
	for _, def := range macroGroupDefsPercussion {
		for _, p := range def.Params {
			for _, pk := range pickers {
				if p == pk {
					if prev, dup := owner[pk]; dup {
						t.Errorf("picker %q claimed by both %q and %q", pk, prev, def.ID)
					}
					owner[pk] = def.ID
				}
			}
		}
	}
	for _, pk := range pickers {
		if _, ok := owner[pk]; !ok {
			t.Errorf("percussion picker %q is not claimed by any concept", pk)
		}
	}
}

// No root concept card may drill into an EMPTY pane: every built macro group must
// map to at least one bound knob for the instrument, or the user taps a card and
// sees nothing. Exercised through the real panel build for a kick.
func TestSynthMacro_NoEmptyDrill(t *testing.T) {
	g := newKickMacroGame(t)
	g.drum.eqPanelZone.SetActiveTab(TabSynth)
	g.drum.eqPanelZone.SetActiveChannel(channel.Of(roster.Kick808))
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())

	if g.drum.instEditorMacros == nil {
		t.Fatal("kick did not activate the macro layer")
	}
	for _, m := range g.drum.instEditorMacros {
		if idxs := g.drum.macroConceptKnobIdxs(m.ID); len(idxs) == 0 {
			t.Errorf("root concept %q drills into an empty pane (no bound knob)", m.ID)
		}
	}
}

// Every shipped instrument gets categories (no instrument falls back to a
// different flow), every category has at least one essential, and every
// visible non-output param is claimed by exactly one category.
func TestMacroGroups_EveryInstrumentHasCategories(t *testing.T) {
	for _, id := range roster.All {
		defs, merged := instSchemaForTest(t, id)
		checkMacroCategories(t, string(id), defs, merged)
	}
	// A WAV-clone document (MODAL / STRIKE / NOISE stages): no shipped
	// instrument carries those stages until one is adopted.
	if err := installDemoModalDoc(roster.TubularBells); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = audio.InstallInstrumentDocForTest(roster.TubularBells, roster.TubularBells) })
	defs, merged := instSchemaForTest(t, roster.TubularBells)
	checkMacroCategories(t, "tubular-bells (modal demo)", defs, merged)
}

func checkMacroCategories(t *testing.T, id string, defs []audio.ParamDef, merged audio.RecipeParams) {
	t.Helper()
	groups := buildMacroGroups(defs, merged)
	if len(groups) == 0 {
		t.Errorf("instrument %s: no categories", id)
		return
	}
	for _, g := range groups {
		if len(g.Essentials) == 0 {
			t.Errorf("instrument %s category %s: no essentials", id, g.ID)
		}
		if g.Label == "" || g.Gloss == "" {
			t.Errorf("instrument %s category %s: empty label/gloss", id, g.ID)
		}
	}
	// Coverage counts every concept, active or not: a HIDE-UNUSED concept's
	// params are still reachable through the pipeline strip's stage drill.
	claimed := map[string]int{}
	for _, g := range buildMacroGroupsAll(defs, merged) {
		for _, p := range g.Params {
			claimed[p]++
		}
	}
	family, familyOK := activeFamilyStage(defs)
	for _, d := range defs {
		if d.Group == audio.SynthHiddenGroup || isSynthEnableParam(d.Name) || d.Name == "gain" ||
			synthParamHiddenInContext(d, family, familyOK, merged) {
			continue
		}
		if claimed[d.Name] != 1 {
			t.Errorf("instrument %s: param %s claimed %d times (want 1)", id, d.Name, claimed[d.Name])
		}
	}
}

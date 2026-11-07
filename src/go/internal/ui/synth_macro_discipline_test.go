package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// A typo or rename would drift the tables from the engine schema.
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

// Exactly once, never fused into a continuous macro.
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

// Every root card maps to at least one bound knob, or a tap shows nothing.
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

// Every instrument gets categories, each with an essential, and every visible non-output param belongs to exactly one.
func TestMacroGroups_EveryInstrumentHasCategories(t *testing.T) {
	for _, id := range roster.All {
		defs, merged := instSchemaForTest(t, id)
		checkMacroCategories(t, string(id), defs, merged)
	}
	// No shipped instrument carries the WAV-clone stages until one is adopted.
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
	// Active or not: a hidden category's params are still reachable through a stage drill.
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

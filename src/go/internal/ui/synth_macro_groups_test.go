package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

func macroIDs(gs []macroGroup) map[string]macroGroup {
	m := map[string]macroGroup{}
	for _, g := range gs {
		m[g.ID] = g
	}
	return m
}

func groupsFor(t *testing.T, recipeID string) []macroGroup {
	t.Helper()
	reg := audio.RecipeRegistrations()[recipeID]
	if reg == nil {
		t.Fatalf("no recipe registration for %q", recipeID)
	}
	return buildMacroGroups(reg.Params, audio.RecipeDefaultParams(recipeID))
}

// A solo bowed string (violin): shows the universal concepts + Movement (vibrato
// LFO on) + Body (resonator on); hides Voice (no formant) and Width (one voice).
func TestMacroGroups_Melodic_ViolinCoreConcepts(t *testing.T) {
	gs := groupsFor(t, "synth-modular-violin")
	ids := macroIDs(gs)
	for _, want := range []string{"sound", "brightness", "attack", "length", "warmth", "movement", "body"} {
		if _, ok := ids[want]; !ok {
			t.Errorf("violin missing expected concept %q (got %v)", want, keysOf(ids))
		}
	}
	if _, ok := ids["voice"]; ok {
		t.Error("violin should NOT show the Voice concept (formant is off)")
	}
	if _, ok := ids["width"]; ok {
		t.Error("violin is solo (voices==1) — should NOT show the Width concept")
	}
	// Every returned concept must own at least one real, present param.
	sch := map[string]bool{}
	for _, d := range audio.RecipeRegistrations()["synth-modular-violin"].Params {
		sch[d.Name] = true
	}
	for _, g := range gs {
		if len(g.Params) == 0 {
			t.Errorf("concept %q has no params", g.ID)
		}
		for _, p := range g.Params {
			if !sch[p] {
				t.Errorf("concept %q references param %q absent from the schema", g.ID, p)
			}
		}
	}
}

// An ensemble string stacks voices → Width + Human Feel appear.
func TestMacroGroups_Melodic_EnsembleShowsWidth(t *testing.T) {
	ids := macroIDs(groupsFor(t, "synth-modular-violin-ensemble"))
	if _, ok := ids["width"]; !ok {
		t.Error("violin-ensemble (voices>1) should show the Width concept")
	}
	if _, ok := ids["humanfeel"]; !ok {
		t.Error("violin-ensemble (voices>1) should show the Human Feel concept")
	}
}

// A vocal instrument (formant on) → Voice appears.
func TestMacroGroups_Melodic_VocalShowsVoice(t *testing.T) {
	ids := macroIDs(groupsFor(t, "synth-modular-voice-alien"))
	if _, ok := ids["voice"]; !ok {
		t.Error("alien choir (formant on) should show the Voice concept")
	}
}

// Percussion/kick recipes fall back to the existing stage UI (nil) in v1.
func TestMacroGroups_Kick_ReturnsNil(t *testing.T) {
	reg := audio.RecipeRegistrations()["synth-modular-kick-808"]
	if reg == nil {
		t.Skip("kick-808 recipe not registered")
	}
	if gs := buildMacroGroups(reg.Params, audio.RecipeDefaultParams("synth-modular-kick-808")); gs != nil {
		t.Errorf("kick should return nil macro groups (fall back to stage UI); got %d groups", len(gs))
	}
}

// Discrete character pickers must each live in exactly one concept.
func TestMacroGroups_PickersEachInOneConcept(t *testing.T) {
	pickers := []string{"osc_type", "filter_type", "body_model", "lfo_target", "formant_vowel", "formant_voice_type", "fm_algorithm"}
	owner := map[string]string{}
	for _, def := range macroGroupDefsMelodic {
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
			t.Errorf("picker %q is not claimed by any concept", pk)
		}
	}
}

func keysOf(m map[string]macroGroup) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

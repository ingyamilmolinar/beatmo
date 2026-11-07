package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

func macroIDs(gs []macroGroup) map[string]macroGroup {
	m := map[string]macroGroup{}
	for _, g := range gs {
		m[g.ID] = g
	}
	return m
}

func groupsFor(t *testing.T, id instrument.ID) []macroGroup {
	t.Helper()
	defs, merged := instSchemaForTest(t, id)
	return buildMacroGroups(defs, merged)
}

// Movement (vibrato) and Body (resonator) on; Voice (no formant) and Width (one voice) hidden.
func TestMacroGroups_Melodic_ViolinCoreConcepts(t *testing.T) {
	gs := groupsFor(t, roster.Violin)
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
	for _, d := range audio.InstrumentParamDefs(roster.Violin) {
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
	ids := macroIDs(groupsFor(t, roster.ViolinEnsemble))
	if _, ok := ids["width"]; !ok {
		t.Error("violin-ensemble (voices>1) should show the Width concept")
	}
	if _, ok := ids["humanfeel"]; !ok {
		t.Error("violin-ensemble (voices>1) should show the Human Feel concept")
	}
}

// A vocal instrument (formant on) → Voice appears.
func TestMacroGroups_Melodic_VocalShowsVoice(t *testing.T) {
	ids := macroIDs(groupsFor(t, roster.VoiceAlien))
	if _, ok := ids["voice"]; !ok {
		t.Error("alien choir (formant on) should show the Voice concept")
	}
}

// Percussion/kick instruments get their OWN concept table (Punch/Boom/Tone/…),
// built from the REAL modular-kick schema (post_*/voice_freq_hz/amp_*/fm_*), not
// the legacy configurable-kick's kick_* params.
func TestBuildMacroGroups_Percussion_Kick808(t *testing.T) {
	defs, merged := instSchemaForTest(t, roster.Kick808)
	groups := buildMacroGroups(defs, merged)
	if len(groups) == 0 {
		t.Fatal("percussion kick should now yield macro groups (was nil)")
	}
	present := map[string]bool{}
	for _, d := range defs {
		present[d.Name] = true
	}
	ids := macroIDs(groups)
	for _, g := range groups {
		if len(g.Params) == 0 {
			t.Errorf("group %q has zero params", g.ID)
		}
		for _, p := range g.Params {
			if !present[p] {
				t.Errorf("group %q param %q not in kick schema", g.ID, p)
			}
		}
	}
	for _, want := range []string{"punch", "boom", "tone", "length", "source"} {
		if _, ok := ids[want]; !ok {
			t.Errorf("expected percussion concept %q, got %v", want, keysOf(ids))
		}
	}
	if _, ok := ids["width"]; ok {
		t.Error("kick-808 has one voice; Width concept must be hidden")
	}
}

// The melodic table is unaffected by the percussion branch.
func TestBuildMacroGroups_Melodic_Unchanged(t *testing.T) {
	ids := macroIDs(groupsFor(t, roster.Violin))
	for _, want := range []string{"sound", "brightness", "attack", "length"} {
		if _, ok := ids[want]; !ok {
			t.Errorf("melodic set regressed: missing %q (got %v)", want, keysOf(ids))
		}
	}
	if _, ok := ids["punch"]; ok {
		t.Error("melodic instrument must not get the percussion Punch concept")
	}
	if _, ok := ids["boom"]; ok {
		t.Error("melodic instrument must not get the percussion Boom concept")
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

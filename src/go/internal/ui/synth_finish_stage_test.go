package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

var finishKnobs = []string{"post_pitch", "post_decay", "post_decay_rate", "post_body",
	"post_brightness", "post_darken", "post_drive", "out_clamp"}

// TestFinishStageShowsEveryPostKnob: the post ops (resample, fade, low boost,
// air, darken, saturate) and the output limit are a visible FINISH stage on a
// percussion AND a melodic instrument (Phase C task C5; Phase B review #5).
func TestFinishStageShowsEveryPostKnob(t *testing.T) {
	for _, inst := range []instrument.ID{roster.Snare, roster.Violin} {
		g := newSynthTabGame(t)
		g.drum.Rows[0].Instrument = inst
		g.drum.eqPanelZone.SetActiveTab(TabSynth)
		g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())
		if !g.drum.SelectSynthSectionByLabel("FINISH") {
			t.Fatalf("%s: no FINISH stage", inst)
		}
		sec := g.drum.synthSelectedSection()
		if sec == nil {
			t.Fatalf("%s: FINISH selected but no section", inst)
		}
		have := map[string]bool{}
		for _, k := range sec.knobIdxs {
			have[g.drum.instEditorBindings[k].def.Name] = true
		}
		for _, p := range finishKnobs {
			if !have[p] {
				t.Errorf("%s: FINISH lacks %s (has %v)", inst, p, have)
			}
		}
	}
}

// TestFinishKnobNamesDoNotCollide: two knobs that share a screen never share a
// display name. A category card mixes stages (Grit holds POST's drive AND
// FINISH's saturate), so FINISH's knobs must not read as a second "Pitch",
// "Drive" or "Decay" there — nor inside the FINISH stage itself.
func TestFinishKnobNamesDoNotCollide(t *testing.T) {
	for _, inst := range []instrument.ID{roster.Kick, roster.Snare, roster.Violin, roster.FmBass, roster.Hihat} {
		defs, merged := instSchemaForTest(t, inst)
		check := func(where string, names []string) {
			seen := map[string]string{}
			for _, n := range names {
				d := synthParamDisplayName(n)
				if other, dup := seen[d]; dup {
					t.Errorf("%s %s: %q names both %s and %s", inst, where, d, other, n)
				}
				seen[d] = n
			}
		}
		for _, g := range buildMacroGroupsAll(defs, merged) {
			check("card "+g.ID, g.Params)
		}
		check("FINISH stage", finishKnobs)
	}
}

package ui

import (
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// On a config-first family instrument the gen-slot knobs ARE the voice and the generic modular stages ship off, so
// each card lists the family knobs first, not knobs of stages that make no sound.
func TestFamilyKnobsLeadTheirCategoryCards(t *testing.T) {
	for _, inst := range []instrument.ID{roster.Snare, roster.FmBass, roster.Hihat, roster.Tom} {
		defs, merged := instSchemaForTest(t, inst)
		for _, g := range buildMacroGroupsAll(defs, merged) {
			seenGeneric := ""
			for _, name := range g.Params {
				_, family := audio.FamilyStageGroupOfParam(name)
				if !family {
					if seenGeneric == "" {
						seenGeneric = name
					}
					continue
				}
				if seenGeneric != "" {
					t.Errorf("%s card %q: family knob %s is listed after the generic %s (params %s)",
						inst, g.ID, name, seenGeneric, strings.Join(g.Params, ","))
					break
				}
			}
		}
	}
}

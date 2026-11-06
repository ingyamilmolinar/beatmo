package ui

import (
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// On a config-first family instrument (snare, kick, hi-hat, tom, FM bass) the
// gen-slot knobs ARE the voice; the generic modular stages a card also claims
// (OSC, FM, unison, formant, LFO) ship switched off on those instruments. Each card
// listed the generic names first, so the snare's Source card opened on Wave /
// Octave / Algorithm / Op Ratios — knobs of stages that make no sound — and its
// Generator came 23rd. The legacy instruments led with their family knob
// (snare_wave first in Source); this restores that: family knobs first.
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

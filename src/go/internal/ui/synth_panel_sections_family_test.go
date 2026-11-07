//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Every recipe with a <family>_wave knob shows it as "Generator" in the leading VOICE card: it is the instrument's
// own voice, separate from the enable-able OSC stage after it.
func TestSynthSections_WaveControlGetsVoiceCard(t *testing.T) {
	checked := 0
	for _, id := range roster.All {
		schema := audio.InstrumentParamDefs(id)
		// The pure modular voice drives its generator from osc_type in OSC.
		if _, ok := activeFamilyStage(schema); !ok {
			continue
		}
		var waveDef *audio.ParamDef
		for _, d := range schema {
			if d.Name == "gen1_wave" {
				dd := d
				waveDef = &dd
				break
			}
		}
		if waveDef == nil {
			t.Errorf("instrument %q: family voice without a gen1_wave ParamDef", id)
			continue
		}
		checked++
		order := synthSectionOrderForSchema(schema)
		if got := sectionForParamDef(*waveDef); got != synthSectionVoice {
			t.Errorf("instrument %q: %s maps to section %v, want VOICE (the generator card)", id, waveDef.Name, got)
		}
		if len(order) == 0 || order[0] != synthSectionVoice {
			t.Errorf("instrument %q: section order %v does not LEAD with the VOICE generator card", id, order)
		}
		if waveDef.Label != "Generator" {
			t.Errorf("instrument %q: %s label = %q, want \"Generator\"", id, waveDef.Name, waveDef.Label)
		}
	}
	if checked == 0 {
		t.Fatal("no family-voice instrument checked — the guard is vacuous")
	}
}

func TestSynthSections_EveryGridControlLandsInVisibleSection(t *testing.T) {
	for _, id := range roster.All {
		schema := audio.InstrumentParamDefs(id)
		order := synthSectionOrderForSchema(schema)
		visible := map[synthSectionID]bool{}
		for _, sec := range order {
			visible[sec] = true
		}
		vals := schemaValues(schema)
		for _, def := range schema {
			if !synthParamIsGridControl(def) || !audio.FlatFieldDeclared(vals, def.Name) {
				continue
			}
			sec := sectionForParamDef(def)
			if !visible[sec] {
				t.Errorf("recipe %q: knob %q maps to section %v which is absent from the section order %v — knob would be invisible",
					id, def.Name, sec, order)
			}
		}
	}
}

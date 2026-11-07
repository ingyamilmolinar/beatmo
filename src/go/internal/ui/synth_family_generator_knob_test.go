package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Every gen-slot family reads gen_wave in C, the voice's generator waveform, so it is shown once, family-agnostically,
// on whichever family plays, and hidden where none is active (melodic modular voices use osc_type).
func TestConfigFirstFamilyInstrumentsOfferTheGeneratorKnob(t *testing.T) {
	offered := func(inst string) (*audio.ParamDef, bool) {
		schema := audio.InstrumentParamDefs(instrument.ID(inst))
		fam, famOK := activeFamilyStage(schema)
		merged := audio.MergedInstrumentParamsRO(instrument.ID(inst))
		for i := range schema {
			if schema[i].Name == "gen1_wave" {
				return &schema[i], synthParamIsGridControlIn(schema[i], fam, famOK, merged)
			}
		}
		return nil, false
	}
	for _, inst := range []instrument.ID{roster.Snare, roster.FmBass, roster.Tom} {
		def, ok := offered(string(inst))
		if def == nil || !ok {
			t.Errorf("%s: gen1_wave is not offered in the Synth tab", inst)
			continue
		}
		if def.Label != "Generator" || len(def.Enum) != 4 {
			t.Errorf("%s: gen1_wave label=%q enum=%v, want \"Generator\" with Sine/Saw/Square/Triangle", inst, def.Label, def.Enum)
		}
		if got := sectionForParamDef(*def); got != synthSectionVoice {
			t.Errorf("%s: gen1_wave routes to section %v, want VOICE (the family's own knobs)", inst, got)
		}
	}

	// The plate never reads it, and the bare voice has osc_type: no generator knob.
	for _, inst := range []instrument.ID{roster.Hihat, roster.Modular} {
		if _, ok := offered(string(inst)); ok {
			t.Errorf("%s: gen1_wave must stay hidden", inst)
		}
	}
}

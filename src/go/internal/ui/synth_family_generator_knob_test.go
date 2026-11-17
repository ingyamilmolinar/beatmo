package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Every gen-slot voice family (kick, tom, snare, cymbal, FM) reads gen_wave in
// C — it is the voice's generator waveform, the knob the legacy recipes offered
// as "Generator" (snare_wave, fm_wave, …). The config-first conversion left
// gen1_wave in the hidden group, so a config-first snare or FM bass lost the
// control entirely. It is surfaced once, family-agnostically, on whichever
// family the recipe plays — and stays hidden where no family is active (the
// melodic modular voices pick their oscillator with osc_type instead).
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

	// The cymbal PLATE (hi-hat) never reads the generator wave — its module
	// type does not declare it, so the knob is gone (spec 2026-10-04 §6: dead
	// knobs disappear). No family at all (the bare voice) → no generator knob
	// either (osc_type owns that job there).
	for _, inst := range []instrument.ID{roster.Hihat, roster.Modular} {
		if _, ok := offered(string(inst)); ok {
			t.Errorf("%s: gen1_wave must stay hidden", inst)
		}
	}
}

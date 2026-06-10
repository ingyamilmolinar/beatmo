//go:build test

package ui

import (
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// TestSynthSections_EveryGridKnobLandsInVisibleSection is the family-knob
// visibility discipline for the native-deprecation migration: every grid-knob
// ParamDef of every registered recipe must map (via sectionForParam) to a
// section that is present in synthSectionOrderForSchema's order for that
// recipe's schema. A knob whose section is missing from the order is silently
// invisible in the Synth tab — buildSynthTab appends knobIdxs only to sections
// that exist (see the `for j := range sections` loop).
//
// Introduced with the FM family knobs (fm_op*_ratio/_depth/_decay route to the
// FM section, which the legacy bespoke 4-card order lacked); kept general so
// every future family (kick/tom/snare/cymbal/bass) is covered automatically.
// TestSynthSections_WaveKnobGetsVoiceCard locks the generator-type knob's
// placement under the Phase-8B unified layout: every recipe with a <family>_wave
// knob presents it in the leading VOICE card — the per-instrument "what makes
// this sound" section — labeled "Generator". The standardized OSC stage (the
// modular oscillator) is a SEPARATE, enable-able stage that sits after VOICE;
// the family generator picker is the instrument's own voice, so it belongs in
// VOICE, which always leads the unified order.
func TestSynthSections_WaveKnobGetsVoiceCard(t *testing.T) {
	for id, reg := range audio.RecipeRegistrations() {
		var waveDef *audio.ParamDef
		for _, d := range reg.Params {
			if strings.HasSuffix(d.Name, "_wave") {
				dd := d
				waveDef = &dd
				break
			}
		}
		order := synthSectionOrderForSchema(reg.Params)
		if waveDef == nil {
			// No generator knob (noise/KS engines, modular voice handles its
			// own osc_type) → no claim about VOICE here.
			continue
		}
		if id == "synth-modular" || id == "synth-modular-pad" {
			continue // modular's osc_type already owns its OSC stage
		}
		if got := sectionForParamDefIn(id, *waveDef); got != synthSectionVoice {
			t.Errorf("recipe %q: %s maps to section %v, want VOICE (the generator card)", id, waveDef.Name, got)
		}
		if len(order) == 0 || order[0] != synthSectionVoice {
			t.Errorf("recipe %q: section order %v does not LEAD with the VOICE generator card", id, order)
		}
		if waveDef.Label != "Generator" {
			t.Errorf("recipe %q: %s label = %q, want \"Generator\"", id, waveDef.Name, waveDef.Label)
		}
	}
}

func TestSynthSections_EveryGridKnobLandsInVisibleSection(t *testing.T) {
	for id, reg := range audio.RecipeRegistrations() {
		order := synthSectionOrderForSchema(reg.Params)
		visible := map[synthSectionID]bool{}
		for _, sec := range order {
			visible[sec] = true
		}
		for _, def := range reg.Params {
			if !synthParamIsGridKnob(def) {
				continue
			}
			sec := sectionForParamDefIn(id, def)
			if !visible[sec] {
				t.Errorf("recipe %q: knob %q maps to section %v which is absent from the section order %v — knob would be invisible",
					id, def.Name, sec, order)
			}
		}
	}
}

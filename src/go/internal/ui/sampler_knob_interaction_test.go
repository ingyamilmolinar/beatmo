//go:build test

package ui

import (
	"testing"
)

func TestSamplerKnobScalesAndBadges(t *testing.T) {
	for _, idx := range []int{samplerKnobStart, samplerKnobEnd, samplerKnobTranspose, samplerKnobDetune, samplerKnobGain} {
		sc := samplerKnobScale(idx)
		if sc.Max <= sc.Min {
			t.Fatalf("knob %d scale invalid: %+v", idx, sc)
		}
	}
	if u := samplerKnobScale(samplerKnobTranspose).Unit; u != "st" {
		t.Fatalf("transpose unit=%q want st", u)
	}
	if u := samplerKnobScale(samplerKnobGain).Unit; u != "dB" {
		t.Fatalf("gain unit=%q want dB", u)
	}

	g := newSamplerTabGame(t)
	layoutSamplerTab(t, g)
	s := &g.drum.sampler
	for i, k := range s.knobs {
		if k == nil {
			t.Fatalf("knob %d nil", i)
		}
		if !k.Endless {
			t.Fatalf("sampler knob %d should be Endless", i)
		}
		if k.StepMul <= 0 {
			t.Fatalf("sampler knob %d StepMul not initialised", i)
		}
		// Badges still back the value-wheel popup's resolution ladder even though
		// the on-card step-badge chrome is retired.
		if i >= len(s.knobStepBadges) || s.knobStepBadges[i] == nil {
			t.Fatalf("sampler knob %d missing step badge", i)
		}
	}
}

//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

func TestSamplerKnobMappingRoundTrips(t *testing.T) {
	assertDefaultParityState(t)
	s := &samplerState{}
	s.reset()

	cases := []struct {
		idx int
		v   float64
	}{
		{samplerKnobStart, 0.3},
		{samplerKnobEnd, 0.9},
		{samplerKnobTranspose, 0.75},
		{samplerKnobDetune, 0.25},
		{samplerKnobGain, 0.6},
	}
	for _, c := range cases {
		s.setKnob(c.idx, c.v)
		if got := s.knobValue(c.idx); got < c.v-1e-6 || got > c.v+1e-6 {
			t.Errorf("knob %d round-trip: set %v, read back %v", c.idx, c.v, got)
		}
	}
}

func TestSamplerTransposeKnobRange(t *testing.T) {
	assertDefaultParityState(t)
	s := &samplerState{}
	s.reset()

	s.setKnob(samplerKnobTranspose, 0.5)
	if s.transposeSemis != 0 {
		t.Errorf("center transpose = %v, want 0", s.transposeSemis)
	}
	s.setKnob(samplerKnobTranspose, 1.0)
	if s.transposeSemis != 24 {
		t.Errorf("max transpose = %v, want +24", s.transposeSemis)
	}
	s.setKnob(samplerKnobTranspose, 0.0)
	if s.transposeSemis != -24 {
		t.Errorf("min transpose = %v, want -24", s.transposeSemis)
	}
}

func TestSamplerEditClampsAndOrdersTrim(t *testing.T) {
	assertDefaultParityState(t)
	s := &samplerState{}
	s.reset()
	s.startFrac = 0.8
	s.endFrac = 0.2
	e := s.edit()
	if e.StartFrac != 0.2 || e.EndFrac != 0.8 {
		t.Errorf("edit trim = [%v,%v], want ordered [0.2,0.8]", e.StartFrac, e.EndFrac)
	}
}

func TestSamplerCaptureFromSynthPopulatesRaw(t *testing.T) {
	assertDefaultParityState(t)
	s := &samplerState{}
	s.reset()
	s.captureFromSynth(roster.Kick)
	if len(s.raw) == 0 {
		t.Fatal("capture produced empty raw buffer")
	}
	if s.rawSampleRate <= 0 {
		t.Errorf("capture rawSampleRate = %d, want > 0", s.rawSampleRate)
	}
	if s.captureID != roster.Kick {
		t.Errorf("captureID = %q, want kick", s.captureID)
	}
	if s.source != samplerSourceSynth {
		t.Errorf("source = %v, want synth", s.source)
	}
}

func TestSamplerBakeTrimsCapturedBuffer(t *testing.T) {
	assertDefaultParityState(t)
	s := &samplerState{}
	s.reset()
	s.captureFromSynth(roster.Kick)
	s.startFrac = 0.0
	s.endFrac = 0.5
	baked, sr := s.bake()
	if sr <= 0 {
		t.Errorf("baked sr = %d, want > 0", sr)
	}
	if len(baked) == 0 {
		t.Fatal("baked buffer empty")
	}
	if len(baked) > len(s.raw) {
		t.Errorf("baked len %d exceeds raw len %d (trim should shorten)", len(baked), len(s.raw))
	}
}

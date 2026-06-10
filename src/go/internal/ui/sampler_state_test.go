//go:build test

package ui

import (
	"sync"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
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
	s.captureFromSynth("kick")
	if len(s.raw) == 0 {
		t.Fatal("capture produced empty raw buffer")
	}
	if s.rawSampleRate <= 0 {
		t.Errorf("capture rawSampleRate = %d, want > 0", s.rawSampleRate)
	}
	if s.captureID != "kick" {
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
	s.captureFromSynth("kick")
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

func TestSamplerSaveAsRegistersNewInstrument(t *testing.T) {
	assertDefaultParityState(t)
	s := &samplerState{}
	s.reset()
	s.captureFromSynth("kick")
	id := s.saveAs("My Chop")
	if id == "" {
		t.Fatal("saveAs returned empty id")
	}
	found := false
	for _, x := range audio.Instruments() {
		if x == id {
			found = true
		}
	}
	if !found {
		t.Errorf("saveAs id %q not registered in audio.Instruments()", id)
	}
}

func TestSamplerSavePublishesHooks(t *testing.T) {
	assertDefaultParityState(t)
	// save() on a synth source stores a global sample-edit descriptor (the
	// recipe binding is kept); restore the global state so later tests see
	// shipped defaults.
	origKickRecipe := audio.RecipeForInstrument("kick")
	t.Cleanup(func() {
		audio.BindInstrumentToRecipe("kick", origKickRecipe)
		audio.ClearSampleEdit("kick")
	})

	var mu sync.Mutex
	var seen []hooks.Event
	for _, k := range []hooks.Kind{hooks.EventSampleSaved, hooks.EventSampleCreated} {
		unsub := hooks.Subscribe(k, func(e hooks.Event) {
			mu.Lock()
			seen = append(seen, e)
			mu.Unlock()
		})
		t.Cleanup(unsub)
	}
	events := func() []hooks.Event {
		mu.Lock()
		defer mu.Unlock()
		out := make([]hooks.Event, len(seen))
		copy(out, seen)
		return out
	}

	s := &samplerState{}
	s.reset()
	s.captureFromSynth("kick")

	s.save()
	e := waitForKind(t, events, hooks.EventSampleSaved)
	p, ok := e.Payload.(hooks.SamplePayload)
	if !ok {
		t.Fatalf("EventSampleSaved payload = %T, want hooks.SamplePayload", e.Payload)
	}
	if p.SampleID != "kick" {
		t.Errorf("saved SampleID = %q, want kick", p.SampleID)
	}

	id := s.saveAs("My Chop")
	e = waitForKind(t, events, hooks.EventSampleCreated)
	p, ok = e.Payload.(hooks.SamplePayload)
	if !ok {
		t.Fatalf("EventSampleCreated payload = %T, want hooks.SamplePayload", e.Payload)
	}
	if p.SampleID != id {
		t.Errorf("created SampleID = %q, want %q", p.SampleID, id)
	}
	if p.DisplayName != "My Chop" {
		t.Errorf("created DisplayName = %q, want My Chop", p.DisplayName)
	}
}

func TestSamplerSaveOverridesCaptureInstrument(t *testing.T) {
	assertDefaultParityState(t)
	// save() clears the recipe binding (converts to a sample); restore it so
	// later tests see the shipped binding.
	origKickRecipe := audio.RecipeForInstrument("kick")
	t.Cleanup(func() { audio.BindInstrumentToRecipe("kick", origKickRecipe) })
	s := &samplerState{}
	s.reset()
	s.captureFromSynth("kick")
	// Save overrides the captured instrument id in place; it must remain a
	// known instrument afterwards.
	s.save()
	found := false
	for _, x := range audio.Instruments() {
		if x == "kick" {
			found = true
		}
	}
	if !found {
		t.Error("after save, captured instrument kick is no longer registered")
	}
}

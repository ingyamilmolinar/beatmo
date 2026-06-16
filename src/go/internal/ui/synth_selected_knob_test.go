package ui

import "testing"

func TestSynthSelectedKnob_DefaultsToSectionMain(t *testing.T) {
	dv := &DrumView{}
	sec := &synthSection{id: synthSectionFilter, knobIdxs: []int{5, 6, 7}}
	if got := dv.synthSelectedKnobIdx("kick", sec); got != 5 {
		t.Fatalf("default selected knob = %d, want 5 (section main)", got)
	}
}

func TestSynthSelectedKnob_StoresAndResolves(t *testing.T) {
	dv := &DrumView{}
	sec := &synthSection{id: synthSectionFilter, knobIdxs: []int{5, 6, 7}}
	dv.setSynthSelectedKnob("kick", 7)
	if got := dv.synthSelectedKnobIdx("kick", sec); got != 7 {
		t.Fatalf("stored selected knob = %d, want 7", got)
	}
}

func TestSynthSelectedKnob_StaleIndexFallsBackToMain(t *testing.T) {
	dv := &DrumView{}
	sec := &synthSection{id: synthSectionFilter, knobIdxs: []int{5, 6, 7}}
	dv.setSynthSelectedKnob("kick", 99)
	if got := dv.synthSelectedKnobIdx("kick", sec); got != 5 {
		t.Fatalf("stale selection should fall back to section main 5, got %d", got)
	}
}

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

func TestSynthSelectedControl_DefaultsToSectionMain(t *testing.T) {
	dv := &DrumView{}
	sec := &synthSection{id: synthSectionFilter, knobIdxs: []int{5, 6, 7}}
	if got := dv.synthSelectedControlIdx(roster.Kick, sec); got != 5 {
		t.Fatalf("default selected knob = %d, want 5 (section main)", got)
	}
}

func TestSynthSelectedControl_StoresAndResolves(t *testing.T) {
	dv := &DrumView{}
	sec := &synthSection{id: synthSectionFilter, knobIdxs: []int{5, 6, 7}}
	dv.setSynthSelectedControl(roster.Kick, 7)
	if got := dv.synthSelectedControlIdx(roster.Kick, sec); got != 7 {
		t.Fatalf("stored selected knob = %d, want 7", got)
	}
}

func TestSynthSelectedControl_StaleIndexFallsBackToMain(t *testing.T) {
	dv := &DrumView{}
	sec := &synthSection{id: synthSectionFilter, knobIdxs: []int{5, 6, 7}}
	dv.setSynthSelectedControl(roster.Kick, 99)
	if got := dv.synthSelectedControlIdx(roster.Kick, sec); got != 5 {
		t.Fatalf("stale selection should fall back to section main 5, got %d", got)
	}
}

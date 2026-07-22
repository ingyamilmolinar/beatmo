//go:build test

package ui

import "testing"

func TestRequestSeekDeltaStepsMovesBySubdivisions(t *testing.T) {
	g := newGameWithStart(t)
	div := g.grid.MaxDiv()
	g.SeekSteps(8*div, seekOriginJS) // start at beat 8
	g.drum.RequestSeekDeltaSteps(3, seekOriginCounter)
	g.drainSeekRequest()
	if got, want := g.playheadAbsSubdiv(), 8*div+3; got != want {
		t.Fatalf("playhead=%d want %d (+3 subdivisions, no beat snap)", got, want)
	}
}

func TestRequestSeekDeltaStepsNegativeClampsAtZero(t *testing.T) {
	g := newGameWithStart(t)
	g.SeekSteps(2, seekOriginJS)
	g.drum.RequestSeekDeltaSteps(-10, seekOriginCounter)
	g.drainSeekRequest()
	if got := g.playheadAbsSubdiv(); got != 0 {
		t.Fatalf("playhead=%d want 0 (clamped)", got)
	}
}

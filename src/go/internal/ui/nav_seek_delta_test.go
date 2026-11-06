//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

func TestRequestSeekDeltaStepsMovesBySubdivisions(t *testing.T) {
	g := newGameWithStart(t)
	div := g.grid.MaxDiv()
	g.SeekSteps(8*div, hooks.SeekJS) // start at beat 8
	g.drum.RequestSeekDeltaSteps(3, hooks.SeekCounter)
	g.drainSeekRequest()
	if got, want := g.playheadAbsSubdiv(), 8*div+3; got != want {
		t.Fatalf("playhead=%d want %d (+3 subdivisions, no beat snap)", got, want)
	}
}

func TestRequestSeekDeltaStepsNegativeClampsAtZero(t *testing.T) {
	g := newGameWithStart(t)
	g.SeekSteps(2, hooks.SeekJS)
	g.drum.RequestSeekDeltaSteps(-10, hooks.SeekCounter)
	g.drainSeekRequest()
	if got := g.playheadAbsSubdiv(); got != 0 {
		t.Fatalf("playhead=%d want 0 (clamped)", got)
	}
}

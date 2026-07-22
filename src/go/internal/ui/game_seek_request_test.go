package ui

import "testing"

func TestSeekRequestAbsoluteDrains(t *testing.T) {
	g := newGameWithStart(t)
	div := g.grid.MaxDiv()
	g.drum.RequestSeekSteps(5*div, seekOriginRibbon)
	g.drainSeekRequest() // the helper the Update drain site calls
	if got := g.playheadAbsSubdiv(); got != 5*div {
		t.Fatalf("playhead=%d want %d", got, 5*div)
	}
	// Request is one-shot.
	if _, _, _, ok := g.drum.TakeSeekRequest(); ok {
		t.Fatal("seek request should be consumed after drain")
	}
}

func TestSeekRequestDeltaResolvesAgainstPlayhead(t *testing.T) {
	g := newGameWithStart(t)
	div := g.grid.MaxDiv()
	g.SeekSteps(8*div, seekOriginJS) // start at beat 8
	g.drum.RequestSeekDeltaBeats(-2, seekOriginTransport)
	g.drainSeekRequest()
	if got := g.playheadAbsSubdiv(); got != 6*div {
		t.Fatalf("playhead=%d want %d (8-2 beats)", got, 6*div)
	}
}

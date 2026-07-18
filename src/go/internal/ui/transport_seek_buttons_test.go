package ui

import "testing"

// TestFastForwardButtonRequestsForwardSeek drives the FF transport button's
// OnClick callback directly and asserts it queues a +jumpBeats (default 4)
// relative seek that lands the playhead jumpBeats ahead after draining.
// Updated from the old fixed ±1 beat to the configurable jump-size feature
// (see TestFastForwardJumpsByDefaultN in transport_jump_test.go for the
// dedicated coverage of the new behavior — kept here too since this file
// predates it and still exercises the same callback path).
func TestFastForwardButtonRequestsForwardSeek(t *testing.T) {
	g := newGameWithStart(t)
	div := g.grid.MaxDiv()
	g.SeekSteps(4*div, seekOriginJS) // start at beat 4
	// Simulate the FF button firing its OnClick.
	g.drum.transportZone.callbacks.OnFastForward() // unexported field, reachable from package ui
	g.drainSeekRequest()
	want := 4 + g.drum.jumpBeats
	if got := g.playheadAbsSubdiv(); got != want*div {
		t.Fatalf("playhead=%d want %d (+%d beats)", got, want*div, g.drum.jumpBeats)
	}
}

// TestRewindButtonRequestsBackwardSeek mirrors the above for the RW button.
func TestRewindButtonRequestsBackwardSeek(t *testing.T) {
	g := newGameWithStart(t)
	div := g.grid.MaxDiv()
	g.SeekSteps(8*div, seekOriginJS) // start at beat 8 (room for the default 4-beat jump)
	g.drum.transportZone.callbacks.OnRewind()
	g.drainSeekRequest()
	want := 8 - g.drum.jumpBeats
	if got := g.playheadAbsSubdiv(); got != want*div {
		t.Fatalf("playhead=%d want %d (-%d beats)", got, want*div, g.drum.jumpBeats)
	}
}

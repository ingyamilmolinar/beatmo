//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// TestStoppedSeekTracksPlayheadWhenLockOn pins the requested behavior: when the
// drum-view tracking lock (FollowPlayback) is ON, seeking while the circuit is
// STOPPED must scroll the drum view to keep the playhead in view. The lock
// means "always follow the playback pointer", not "follow only during
// playback". Before the fix, updateDrumTracking only calls TrackBeat while
// g.Playing(), so a stopped seek leaves the drum view behind.
func TestStoppedSeekTracksPlayheadWhenLockOn(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.SetPlayFunc(func(instrument.ID, float64, ...float64) {})
	g.Layout(1024, 720)
	g.drum.SetLength(16)
	g.drum.SetBPM(120)
	g.drum.SetFollow(true) // tracking lock ON
	g.drum.Offset = 0

	div := g.grid.MaxDiv()
	// Circuit is STOPPED (play never pressed). Seek far ahead so the playhead
	// leaves the initial window and tracking must scroll to keep it visible.
	g.SeekSteps(40*div, hooks.SeekRibbon)
	g.updateDrumTracking()

	head := g.playheadAbsSubdiv()
	if head != 40*div {
		t.Fatalf("playhead=%d want %d after stopped seek", head, 40*div)
	}
	// With the lock ON while stopped, the drum view must have scrolled so the
	// playhead sits inside the visible window [Offset, Offset+Length).
	if !(g.drum.Offset > 0 && head >= g.drum.Offset && head < g.drum.Offset+g.drum.Length) {
		t.Fatalf("lock ON + stopped: playhead %d not tracked into window [%d,%d) (Offset=%d Length=%d)",
			head, g.drum.Offset, g.drum.Offset+g.drum.Length, g.drum.Offset, g.drum.Length)
	}
}

// TestStoppedSeekDoesNotTrackWhenLockOff guards the negative case: with the
// lock OFF, a stopped seek must NOT move the drum view — the user stays free to
// scroll the editing window independently of the playhead.
func TestStoppedSeekDoesNotTrackWhenLockOff(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.SetPlayFunc(func(instrument.ID, float64, ...float64) {})
	g.Layout(1024, 720)
	g.drum.SetLength(16)
	g.drum.SetBPM(120)
	g.drum.SetFollow(false) // tracking lock OFF
	g.drum.Offset = 0

	div := g.grid.MaxDiv()
	g.SeekSteps(40*div, hooks.SeekRibbon)
	g.updateDrumTracking()

	if g.drum.Offset != 0 {
		t.Fatalf("lock OFF + stopped: drum view must not track; Offset moved to %d", g.drum.Offset)
	}
}

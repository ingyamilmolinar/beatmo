//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// The tracking lock means "always follow the playback pointer", so a seek while STOPPED scrolls the view too.
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
	g.SeekSteps(40*div, hooks.SeekRibbon)
	g.updateDrumTracking()

	head := g.playheadAbsSubdiv()
	if head != 40*div {
		t.Fatalf("playhead=%d want %d after stopped seek", head, 40*div)
	}
	if !(g.drum.Offset > 0 && head >= g.drum.Offset && head < g.drum.Offset+g.drum.Length) {
		t.Fatalf("lock ON + stopped: playhead %d not tracked into window [%d,%d) (Offset=%d Length=%d)",
			head, g.drum.Offset, g.drum.Offset+g.drum.Length, g.drum.Offset, g.drum.Length)
	}
}

// With the lock off the user scrolls the editing window independently of the playhead.
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

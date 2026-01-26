package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/tunkul/core/model"
)

// Verifies that pressing pause does not advance the highlighted subdivision
// even by a single step; the marker remains fixed for the pause frame.
func TestPauseDoesNotAdvanceHighlight(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	w, h := 640, 480
	g.Layout(w, h)
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return w, h },
	)
	defer restore()

	// Simple 1-beat segment
	div := g.grid.MaxDiv()
	n0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n1 := g.tryAddNode(div, 0, model.NodeTypeRegular)
	g.addEdge(n0, n1)
	g.updateBeatInfos()

	g.drum.SetBPM(120)
	g.SetAppliedBPMForTest(120)

	pressPlay(t, g.drum)
	_ = g.Update()
	// Advance deterministically.
	setPlayStartForAbs(g, div+1)
	_ = g.Update()

	// Snapshot current subdivision and highlight
	prev := g.elapsedBeats

	// Pause now
	pressPlay(t, g.drum)
	_ = g.Update()

	if g.elapsedBeats != prev {
		t.Fatalf("pause advanced elapsedBeats: %d -> %d", prev, g.elapsedBeats)
	}
	// Ensure the highlight map reflects the same position, not prev+1.
	if _, ok := g.highlightedBeats[makeBeatKey(0, prev)]; !ok {
		t.Fatalf("highlight missing at paused position %d", prev)
	}
	if _, ok := g.highlightedBeats[makeBeatKey(0, prev+1)]; ok {
		t.Fatalf("highlight incorrectly advanced to %d on pause", prev+1)
	}
}

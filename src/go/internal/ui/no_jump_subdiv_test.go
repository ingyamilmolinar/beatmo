package ui

import (
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/tunkul/core/model"
)

// Ensure that between successive UI updates, the highlighted subdivision does
// not advance by more than one step when time-based sync is active.
func TestNeverJumpSubdivPerFrame(t *testing.T) {
	g := New(testLogger)
	g.SetUseSequencerForTest(true)
	g.Layout(800, 600)
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	// 1-beat horizontal segment
	beat := g.grid.MaxDiv()
	n0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n1 := g.tryAddNode(beat, 0, model.NodeTypeRegular)
	g.addEdge(n0, n1)
	g.updateBeatInfos()

	g.drum.SetBPM(120)
	until := time.Now().Add(30 * time.Millisecond)
	for time.Now().Before(until) {
		_ = g.Update()
		time.Sleep(5 * time.Millisecond)
	}
	g.drum.playPressed = true

	// Run for ~200ms. Per Update, delta of elapsedBeats must be <= 1.
	last := g.elapsedBeats
	start := time.Now()
	for time.Since(start) < 200*time.Millisecond {
		_ = g.Update()
		cur := g.elapsedBeats
		if cur < last {
			t.Fatalf("elapsedBeats moved backwards: %d -> %d", last, cur)
		}
		if cur-last > 1 {
			t.Fatalf("elapsedBeats jumped by >1: %d -> %d", last, cur)
		}
		last = cur
		time.Sleep(5 * time.Millisecond)
	}
}

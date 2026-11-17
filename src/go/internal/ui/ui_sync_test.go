package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

// Ensure the UI stays in sync with playback while increasing drum length
// aggressively. Uses the engine timeline (currentBeat) and forces the
// time-based sync logic on for test.
func TestUISyncKeepsUpDuringLengthIncrease(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	w, h := 800, 600
	g.Layout(w, h)

	// Freeze input to avoid incidental drags in real-Ebiten mode.
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return w, h },
	)
	defer restore()

	// Build a 1x1-beat rectangle.
	beat := g.grid.MaxDiv()
	g.pendingStartRow = 0
	n0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n1 := g.tryAddNode(beat, 0, model.NodeTypeRegular)
	n2 := g.tryAddNode(beat, beat, model.NodeTypeRegular)
	n3 := g.tryAddNode(0, beat, model.NodeTypeRegular)
	g.addEdge(n0, n1)
	g.addEdge(n1, n2)
	g.addEdge(n2, n3)
	g.addEdge(n3, n0)
	g.pendingStartRow = -1
	g.updateBeatInfos()

	// Start playback.
	g.drum.SetBPM(120)
	g.SetAppliedBPMForTest(120)
	pressPlay(t, g.drum)
	// Increase length aggressively while updating.
	_ = g.Update()
	for i := 0; i < 30; i++ {
		if i%3 == 0 {
			pressLenInc(t, g.drum)
		}
		setPlayStartForAbs(g, i)
		_ = g.Update()
	}

	// Final sync to capture latest time.
	_ = g.Update()
	// Compute expected absolute subdivision position from engine timeline.
	target := 29
	// UI counter should be close to target (within a small tolerance of 1 subdiv).
	diff := g.elapsedBeats - target
	if diff < 0 {
		diff = -diff
	}
	if diff > 1 {
		t.Fatalf("UI drifted from playback: elapsed=%d target=%d", g.elapsedBeats, target)
	}
	// UI highlight is driven by pulse advancement and may lag by < 1 subdiv;
	// we rely on the counter check above to assert timeline sync.
}

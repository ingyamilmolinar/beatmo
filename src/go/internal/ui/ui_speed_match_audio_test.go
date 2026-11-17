package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

// Verifies that under the time-based visual path, the UI subdivision index
// (elapsedBeats) keeps up with the audio timebase within 1 subdivision.
func TestUISpeedMatchesAudioNoLag(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	w, h := 800, 600
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

	// Build a square so there are multiple segments but consistent speed.
	div := g.grid.MaxDiv()
	n0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n1 := g.tryAddNode(div, 0, model.NodeTypeRegular)
	n2 := g.tryAddNode(div, div, model.NodeTypeRegular)
	n3 := g.tryAddNode(0, div, model.NodeTypeRegular)
	g.addEdge(n0, n1)
	g.addEdge(n1, n2)
	g.addEdge(n2, n3)
	g.addEdge(n3, n0)
	g.updateBeatInfos()

	// Faster tempo to stress catch-up such that per-frame delta can exceed 1 subdiv.
	g.drum.SetBPM(240)
	g.SetAppliedBPMForTest(240)

	// Start playback and allow one frame to establish timebase in UI.
	pressPlay(t, g.drum)
	_ = g.Update()
	tol := 1 // subdivisions
	for abs := 0; abs < div*2; abs++ {
		setPlayStartForAbs(g, abs)
		_ = g.Update()
		expected := abs
		lag := expected - g.elapsedBeats
		if lag > tol {
			t.Fatalf("UI lag behind audio: expected=%d got=%d lag=%d", expected, g.elapsedBeats, lag)
		}
	}
}

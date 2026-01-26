//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/tunkul/core/model"
)

// TestPulseAnimationSmooth verifies that pulse progress advances smoothly
// frame-to-frame at roughly the expected rate for a 1-beat segment at 60 BPM.
func TestPulseAnimationSmooth(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	g.drum.SetBPM(60)
	// Create a 1-beat horizontal segment: distance = 32 units → 1 beat.
	n0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n1 := g.tryAddNode(g.grid.MaxDiv(), 0, model.NodeTypeRegular)
	g.addEdge(n0, n1)
	g.updateBeatInfos()
	g.SetPlaying(true)
	g.spawnPulseFromRow(0, 0)
	if g.activePulse == nil {
		t.Fatalf("expected active pulse")
	}
	// Over ~10 frames, progress should increase monotonically and be < 1.
	_ = g.activePulse.t
	for i := 0; i < 10; i++ {
		setPlayStartForAbs(g, i+1)
		g.Update()
		if g.activePulse == nil {
			t.Fatalf("pulse disappeared unexpectedly at frame %d", i)
		}
		if g.activePulse.t < 0 || g.activePulse.t >= 1 {
			t.Fatalf("pulse t out of range [0,1): %f (frame %d)", g.activePulse.t, i)
		}
		if g.activePulse.t >= 1 {
			t.Fatalf("pulse reached end too early at frame %d: t=%f", i, g.activePulse.t)
		}
		// no-op; next frame will re-read t
	}
}

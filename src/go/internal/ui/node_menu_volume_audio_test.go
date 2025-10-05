package ui

import (
	"testing"
	"time"

	"github.com/ingyamilmolinar/tunkul/core/model"
)

// TestNodeMenuVolumePercentAdjustsAudio opens the popup, clicks VOL- once
// (100% -> 90%), and verifies the next playback uses the reduced volume.
func TestNodeMenuVolumePercentAdjustsAudio(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	g.SetUseSequencerForTest(false)

	// Simple two-node path
	n0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n1 := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(n0, n1)

	// Open node menu on n1
	g.sel = n1
	n1.Selected = true
	g.nodeMenuOpen = true
	g.nodeMenuNode = n1
	g.updateNodeMenuRects()
	if r := g.nodeMenuRects["vol-"]; r.Empty() {
		t.Fatalf("vol- rect missing")
	}
	btn := g.nodeMenuBtns["vol-"]
	if btn == nil {
		t.Fatalf("vol- button missing")
	}
	// Click once: 100% -> 90%
	btn.OnClick()
	if err := g.Update(); err != nil {
		t.Fatalf("update: %v", err)
	}

	// Capture volumes when playing across the two nodes.
	vols := make(chan float64, 2)
	g.SetPlayFunc(func(_ string, v float64, _ ...float64) { vols <- v })

	g.playing = true
	g.spawnPulseFrom(0)
	v0 := <-vols // n0 baseline
	// Advance to n1
	g.activePulse.t = 1
	g.Update()
	var v1 float64
	select {
	case v1 = <-vols:
	case <-time.After(50 * time.Millisecond):
		t.Fatalf("no playback at n1")
	}

	// Expect approx 0.9x vs baseline
	if v0 <= 0 || v1 <= 0 {
		t.Fatalf("zero volumes v0=%.3f v1=%.3f", v0, v1)
	}
	want := 0.9 * v0
	// allow tiny float error
	if (v1-want) > 1e-6 || (want-v1) > 1e-6 {
		t.Fatalf("volume percent not applied: want 90%% of %.3f => %.3f got %.3f", v0, want, v1)
	}
}

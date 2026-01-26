package ui

import (
	"testing"

	"github.com/ingyamilmolinar/tunkul/core/model"
)

// TestNodeMenuVolumePercentAdjustsAudio opens the popup, clicks VOL- once
// (100% -> 90%), and verifies the next playback uses the reduced volume.
func TestNodeMenuVolumePercentAdjustsAudio(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	// Simple two-node path
	n0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n1 := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(n0, n1)
	g.updateBeatInfos()

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

	g.SetPlaying(true)
	scheduleAbsForMuteTest(g, 0)
	v0 := waitForChan(t, vols, 10000)
	scheduleAbsForMuteTest(g, 1)
	v1 := waitForChan(t, vols, 10000)

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

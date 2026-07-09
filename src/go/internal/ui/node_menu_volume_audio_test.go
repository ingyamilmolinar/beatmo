package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// TestNodeMenuVolumePercentAdjustsAudio opens the popup, clicks VOL- once
// (one perceptual −3 dB step, i.e. ÷nodeVolStepFactor), and verifies the next
// playback uses the reduced volume.
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
	g.sidebar.Open(n1)
	g.sidebar.ExpandAllSections()
	g.sidebar.layout()
	if r := g.sidebar.rects["vol-"]; r.Empty() {
		t.Fatalf("vol- rect missing")
	}
	btn := g.sidebar.btns["vol-"]
	if btn == nil {
		t.Fatalf("vol- button missing")
	}
	// Click once: one perceptual step down (−3 dB).
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
	want := v0 / nodeVolStepFactor
	// allow tiny float error
	if (v1-want) > 1e-6 || (want-v1) > 1e-6 {
		t.Fatalf("volume step not applied: want %.3f (−3 dB of %.3f) got %.3f", want, v0, v1)
	}
}

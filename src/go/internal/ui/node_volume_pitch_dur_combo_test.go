package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// Repro: Changing volume on a node that also has pitch and duration set must
// still affect playback volume. This ensures the three parameters combine.
func TestNodeVolumeWorksWithPitchAndDuration(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.updateBeatInfos()

	// Apply pitch and duration to the second node along with a loud volume.
	g.graph.SetNodeParams(b.ID, model.NodeParams{Volume: 1.5, Pitch: +3, Duration: 0.75})

	vols := make(chan float64, 2)
	g.SetPlayFunc(func(_ string, v float64, _ ...float64) { vols <- v })

	g.SetPlaying(true)
	scheduleAbsForMuteTest(g, 0)
	v0 := waitForChan(t, vols, 10000)
	scheduleAbsForMuteTest(g, 1)
	v1 := waitForChan(t, vols, 10000)

	if v0 <= 0 || v1 <= 0 {
		t.Fatalf("unexpected zero volumes: v0=%.3f v1=%.3f", v0, v1)
	}
	// Expect boosted volume (1.5x); allow small float tolerance
	want := 1.5 * v0
	if (v1-want) > 1e-6 || (want-v1) > 1e-6 {
		t.Fatalf("volume not combined with pitch/duration: want %.3f got %.3f (base %.3f)", want, v1, v0)
	}
}

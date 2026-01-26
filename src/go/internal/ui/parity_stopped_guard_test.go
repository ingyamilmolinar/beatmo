package ui

import (
	"testing"

	"github.com/ingyamilmolinar/tunkul/core/model"
)

// Parity scan should ignore checks after playback is stopped so stale audio
// events/highlights collected during the last frame cannot trigger panics.
func TestParityScanSkippedWhenNotPlaying(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	// Not playing: simulate a stale audio event without a highlight.
	g.SetPlaying(false)
	root := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.start = root
	g.graph.StartNodeID = root.ID
	g.drum.Rows[0].Origin = root.ID
	g.drum.Rows[0].Node = root
	g.addEdge(root, root)
	g.updateBeatInfos()
	g.drum.SetLength(1)
	g.drum.Offset = 0
	g.refreshDrumRow()
	g.recordParityAudio(0, 0, 0.1, "kick", 1, 0, 1, g.audioGen.Load())

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("parityScan should not panic when not playing; got %v", r)
		}
	}()
	g.parityScan("stopped")

	if got := len(g.ParityMismatchSnapshot()); got != 0 {
		t.Fatalf("expected no parity mismatches when not playing, got %d", got)
	}
}

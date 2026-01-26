package ui

import (
	"testing"

	"github.com/ingyamilmolinar/tunkul/core/model"
)

// Regression: parity scans must ignore audio events from prior audio generations
// (e.g., stop/replay) so stale audio doesn't trigger highlight/audio panics.
func TestParityIgnoresStaleAudioGeneration(t *testing.T) {
	assertDefaultParityState(t)
	g := buildTestGame(t)
	t.Cleanup(g.CloseForTest)

	g.drum.SetLength(4)

	// Build a simple path but omit node snapshots so predictor treats beats as inaudible.
	paths := [][]model.BeatInfo{make([]model.BeatInfo, 4)}
	for i := range paths[0] {
		paths[0][i] = model.BeatInfo{NodeID: 1, NodeType: model.NodeTypeRegular}
	}
	g.beatInfosByRow = paths
	g.isLoopByRow = []bool{false}
	g.loopStartByRow = []int{0}
	g.engine.Predictor.SetPaths(paths, g.isLoopByRow, g.loopStartByRow, map[model.NodeID]model.Node{})
	g.pathsDirty = false
	g.drum.Offset = 0
	g.refreshDrumRow()

	// Activate parity checks after setup to avoid setup-time panics.
	g.parityWatch = parityWatchPanic
	g.ClearParityMismatches()

	// Simulate a running transport where abs=0 is the current beat.
	g.nextBeatIdxs = []int{1}
	g.seqNextIdxs = []int{1}
	g.elapsedBeats = 0
	g.SetPlaying(true)

	// Bump audio generation and insert a stale audio event.
	g.audioGen.Add(1)
	staleGen := g.audioGen.Load() - 1
	g.recordParityAudio(0, 0, 0, "kick", 1, 0, 1, staleGen)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("parity scan panicked on stale audio gen: %v", r)
		}
	}()
	g.parityScan("test-stale-audio-gen")
	if got := len(g.ParityMismatchSnapshot()); got != 0 {
		t.Fatalf("expected no parity mismatches from stale audio gen, got %d", got)
	}
}

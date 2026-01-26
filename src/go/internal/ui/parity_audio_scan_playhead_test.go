package ui

import (
	"testing"

	"github.com/ingyamilmolinar/tunkul/core/model"
)

// Regression: audio parity checks must be anchored to the playhead, not the
// currently rendered DrumView offset. Otherwise panning the DrumView can hide
// audio↔predictor mismatches (no panic, no mismatch ring entry).
func TestParityDetectsAudioMismatchWhenViewIsPannedAway(t *testing.T) {
	assertDefaultParityState(t)
	g := buildTestGame(t)
	t.Cleanup(g.CloseForTest)

	g.parityWatch = parityWatchLog
	g.ClearParityMismatches()

	g.drum.SetLength(8)

	// Predictor path exists, but the node snapshot is empty, so the predictor
	// should treat all beats as inaudible/invisible.
	pathLen := 256
	paths := [][]model.BeatInfo{make([]model.BeatInfo, pathLen)}
	for i := range paths[0] {
		paths[0][i] = model.BeatInfo{NodeID: 1, NodeType: model.NodeTypeRegular}
	}
	g.beatInfosByRow = paths
	g.isLoopByRow = []bool{false}
	g.loopStartByRow = []int{0}
	g.engine.Predictor.SetPaths(paths, g.isLoopByRow, g.loopStartByRow, map[model.NodeID]model.Node{})
	g.pathsDirty = false

	// Simulate a running transport where current beat is abs=4.
	g.nextBeatIdxs = []int{5}
	g.seqNextIdxs = []int{5}
	g.elapsedBeats = 4
	g.SetPlaying(true)

	abs := 4
	g.recordSeqDecision(0, abs, true, model.NodeTypeRegular, false)
	g.recordParityAudio(0, abs, 1.0, "kick", 1, 0, 1, g.audioGen.Load())

	// Pan DrumView far away from the playhead so the rendered window doesn't
	// include abs=4.
	g.drum.Offset = 200
	g.refreshDrumRow()

	g.parityScan("test-panned-away")

	found := false
	for _, m := range g.ParityMismatchSnapshot() {
		if m.Kind == "audio_unexpected" && m.Row == 0 && m.Abs == abs {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected audio_unexpected mismatch even when DrumView is panned away; got=%v", g.ParityMismatchSnapshot())
	}
}

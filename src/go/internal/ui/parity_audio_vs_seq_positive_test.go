package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// The true positive: an audio event at the current beat with no seq decision behind it still fires
// audio_vs_seq.
func TestParityDetectsAudioWithoutSeqDecision(t *testing.T) {
	assertDefaultParityState(t)
	g := buildTestGame(t)
	t.Cleanup(g.CloseForTest)

	g.parityWatch = parityWatchLog
	g.ClearParityMismatches()

	g.drum.SetLength(8)

	paths := [][]model.BeatInfo{make([]model.BeatInfo, 8)}
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

	// nextBeatIdxs=4 makes abs=3 the current beat the audio/seq checks anchor on; elapsedBeats=3 keeps the global
	// playhead clamp from lowering it.
	g.nextBeatIdxs = []int{4}
	g.seqNextIdxs = []int{4}
	g.elapsedBeats = 3
	g.SetPlaying(true)

	const abs = 3
	// Only an audio event, at the current generations so the filters keep it.
	g.recordParityAudio(0, abs, 1.0, roster.Kick, 1, 0, 1, g.audioGen.Load(), g.parityGen.Load())
	g.parityScan("test-audio-vs-seq-positive")

	found := false
	for _, m := range g.ParityMismatchSnapshot() {
		if m.Row == 0 && m.Abs == abs && m.Kind == "audio_vs_seq" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected audio_vs_seq to fire for an audio event with no seq decision at abs=%d, got=%v", abs, g.ParityMismatchSnapshot())
	}
}

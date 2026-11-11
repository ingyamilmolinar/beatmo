package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// TestParityDetectsAudioWithoutSeqDecision is the true-positive counterpart to
// TestParityAudioCarriesSchedulingGenAcrossMutation
// (parity_audio_sched_gen_test.go): that test proves audio_vs_seq must NOT
// fire for a legitimately scheduled sound whose seq decision briefly lags a
// generation bump. This test proves the checker still fires audio_vs_seq for
// a genuine bug: an audio event recorded at the current beat with no seq
// decision behind it at all (row_parity_diff_scan.go's
// `if hasAudio { if dec, ok2 := decRow[abs]; !ok2 || !dec.Audible { ... } }`
// branch).
func TestParityDetectsAudioWithoutSeqDecision(t *testing.T) {
	assertDefaultParityState(t)
	g := buildTestGame(t)
	t.Cleanup(g.CloseForTest)

	g.parityWatch.Store(parityWatchLog)
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

	// Advance the playhead so abs=3 is the current beat: nextBeatIdxs[row]=4
	// makes rowPastExclusive()=4 (elapsedBeats=3 keeps the global playhead
	// clamp at >=4 so it doesn't lower it), and audioFloor = pastExclusive-1
	// = 3 is exactly the "current beat" the audio/seq checks anchor on.
	g.nextBeatIdxs = []int{4}
	g.seqNextIdxs = []int{4}
	g.elapsedBeats = 3
	g.SetPlaying(true)

	const abs = 3
	// Deliberately record NO seq decision at abs — only an audio event, at the
	// current parity/audio generation so the gen filters keep it. This is the
	// genuine "audio fired with no audible decision behind it" bug that
	// audio_vs_seq exists to catch.
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

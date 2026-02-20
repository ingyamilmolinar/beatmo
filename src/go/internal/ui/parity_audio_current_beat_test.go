package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// Regression: parityScan should catch audio-vs-predictor/view mismatches on the
// current beat (abs == nextBeatIdxs[row]-1). Previously, audio checks were
// gated by `abs < playhead || inPast`, which skips the current beat because it
// is always in the "true past" (abs < nextBeatIdxs[row]) once highlighted.
func TestParityDetectsAudioMismatchOnCurrentBeat(t *testing.T) {
	assertDefaultParityState(t)
	g := buildTestGame(t)
	t.Cleanup(g.CloseForTest)

	g.parityWatch = parityWatchLog
	g.ClearParityMismatches()

	g.drum.SetLength(8)

	// Predictor says "off" (node missing from snapshot), but sequencer/audio claim a hit.
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

	// Simulate a running transport where next beat is 5, so current beat is 4.
	g.nextBeatIdxs = []int{5}
	g.seqNextIdxs = []int{5}
	g.elapsedBeats = 4
	g.SetPlaying(true)

	abs := 4
	g.recordSeqDecision(0, abs, true, model.NodeTypeRegular, false)
	g.recordParityAudio(0, abs, 1.0, "kick", 1, 0, 1, g.audioGen.Load()) // future time; skip highlight parity in tests
	g.parityScan("test-current-audio")

	foundAudioVsView := false
	foundAudioUnexpected := false
	for _, m := range g.ParityMismatchSnapshot() {
		if m.Row != 0 || m.Abs != abs {
			continue
		}
		if m.Kind == "audio_vs_view" {
			foundAudioVsView = true
		}
		if m.Kind == "audio_unexpected" {
			foundAudioUnexpected = true
		}
	}
	if !foundAudioVsView || !foundAudioUnexpected {
		t.Fatalf("expected parity to catch current-beat audio mismatches (audio_vs_view=%v audio_unexpected=%v), got=%v", foundAudioVsView, foundAudioUnexpected, g.ParityMismatchSnapshot())
	}
}

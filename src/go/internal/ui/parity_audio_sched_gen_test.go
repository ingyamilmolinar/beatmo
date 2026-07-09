package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// Regression for TestBPMChangeDuringLoopKeepsForwardProgress flaking under
// full-suite (make test-real) load.
//
// A sound is scheduled (seq decision recorded + soundReq enqueued) at parity
// generation N. Under load the audioLoop goroutine lags, so it only records the
// matching parity audio event AFTER a structural mutation (a BPM change bumps
// parityGen to N+1). The parity gen-filter keeps entries at the CURRENT
// generation and drops older ones — symmetrically on both the seq-decision and
// audio side (game_parity_diff_scan.go). If the audio event is stamped with the
// record-time generation (N+1) instead of the scheduling generation (N), the
// filter keeps the audio but drops its decision, leaving a phantom
// audio_vs_seq mismatch. Post-mutation grace is bypassed under `go test`
// (parityInGrace), so the gen-filter is the only defense and must hold.
func TestParityAudioCarriesSchedulingGenAcrossMutation(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.start = a
	g.graph.StartNodeID = a.ID
	g.addEdge(a, b)
	g.addEdge(b, a)
	g.updateBeatInfos()

	g.parityWatch = parityWatchLog
	g.drum.SetLength(8)
	g.drum.Offset = 0
	g.refreshDrumRow()

	// Running transport: next beat 5, so the current beat is abs=4.
	g.nextBeatIdxs = []int{5}
	g.seqNextIdxs = []int{5}
	g.elapsedBeats = 4
	g.SetPlaying(true)

	const abs = 4
	// Decision + schedule happen at the current generation.
	schedGen := g.parityGen.Load()
	g.recordSeqDecision(0, abs, true, model.NodeTypeRegular, false)

	// A structural mutation (e.g. BPM change) bumps parityGen before the
	// lagging audioLoop records the already-scheduled sound.
	g.parityGen.Add(1)

	g.ClearParityMismatches()
	// audioLoop records the lagged sound now. It must inherit the scheduling
	// generation (schedGen), not the current one.
	g.recordParityAudio(0, abs, 1.0, "kick", 1, 0, 1, g.audioGen.Load(), schedGen)

	g.parityScan("test-audio-sched-gen")

	for _, m := range g.ParityMismatchSnapshot() {
		if m.Row == 0 && m.Abs == abs && m.Kind == "audio_vs_seq" {
			t.Fatalf("phantom audio_vs_seq: a sound scheduled at parityGen %d before a "+
				"structural mutation was compared against the post-mutation generation; "+
				"audio events must inherit the scheduling generation. got=%+v", schedGen, m)
		}
	}
}

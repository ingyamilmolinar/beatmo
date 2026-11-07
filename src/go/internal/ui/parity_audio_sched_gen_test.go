package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// A lagging audioLoop records a sound scheduled at generation N after a mutation bumped parityGen to N+1. Both
// sides are filtered by the current generation, so the audio event must carry its SCHEDULING generation, or the
// filter keeps the audio, drops its decision and reports a phantom audio_vs_seq; grace is off under go test.
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

	// The mutation bumps parityGen before the lagging audioLoop records the sound.
	g.parityGen.Add(1)

	g.ClearParityMismatches()
	// It must inherit schedGen, not the current generation.
	g.recordParityAudio(0, abs, 1.0, roster.Kick, 1, 0, 1, g.audioGen.Load(), schedGen)

	g.parityScan("test-audio-sched-gen")

	for _, m := range g.ParityMismatchSnapshot() {
		if m.Row == 0 && m.Abs == abs && m.Kind == "audio_vs_seq" {
			t.Fatalf("phantom audio_vs_seq: a sound scheduled at parityGen %d before a "+
				"structural mutation was compared against the post-mutation generation; "+
				"audio events must inherit the scheduling generation. got=%+v", schedGen, m)
		}
	}
}

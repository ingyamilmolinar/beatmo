package ui

import (
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// A decision and its audio enqueue happen under the same seqMu, so audio still queued in audioCh past the grace
// window is late, not lost; only an Audible decision that never enqueued is missing audio.
func TestParityIgnoresInFlightAudioForEnqueuedDecision(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	// Build a simple 2-node loop so abs=0 is a regular/audible step.
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.start = a
	g.graph.StartNodeID = a.ID
	g.addEdge(a, b)
	g.addEdge(b, a)
	g.updateBeatInfos()

	g.parityWatch = parityWatchLog
	g.drum.SetLength(1)
	g.drum.Offset = 0
	g.refreshDrumRow()

	// abs=0 sits at the missing-audio floor (current beat - 1).
	g.nextBeatIdxs = []int{2} // pastExclusive=2 => current beat abs=1
	g.seqNextIdxs = []int{2}

	g.parityMu.Lock()
	g.parityAudio = nil
	g.paritySeqDecisions = make(map[int]map[int]paritySeqDecision)
	g.paritySeqDecisions[0] = map[int]paritySeqDecision{
		0: {
			Row:      0,
			Abs:      0,
			Audible:  true,
			NodeType: model.NodeTypeRegular,
			Missing:  false,
			// Enqueued but not yet dispatched: not a missing-audio violation.
			Enqueued:   true,
			ParityGen:  g.parityGen.Load(),
			RecordedAt: time.Unix(0, 0).Add(-250 * time.Millisecond), // past the grace window
		},
	}
	g.parityMu.Unlock()

	g.ClearParityMismatches()
	g.parityScan("test-audio-inflight")

	for _, m := range g.ParityMismatchSnapshot() {
		if m.Kind == "audio_missing" && m.Row == 0 && m.Abs == 0 {
			t.Fatalf("audio_missing fired for in-flight (enqueued, not yet dispatched) audio; "+
				"this is the startup false-positive that panicked the app: %+v", m)
		}
	}
}

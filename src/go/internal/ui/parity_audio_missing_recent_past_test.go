package ui

import (
	"testing"
	"time"

	"github.com/ingyamilmolinar/tunkul/core/model"
)

// Regression: parityScan's audio_missing check must not be skipped simply
// because the beat became "past" while waiting out the 120ms grace window.
// Otherwise, missing-audio events at ~125ms/step cadences can evade detection.
func TestParityDetectsMissingAudioJustBehindPlayhead(t *testing.T) {
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

	// Simulate the UI playhead being two steps ahead (so abs=0 is now behind the
	// "current beat" floor) while the sequencer previously decided abs=0 should
	// have been audible but no audio event was recorded.
	g.nextBeatIdxs = []int{2} // pastExclusive=2 => current beat abs=1
	g.seqNextIdxs = []int{2}

	g.parityMu.Lock()
	g.parityAudio = nil
	g.paritySeqDecisions = make(map[int]map[int]paritySeqDecision)
	g.paritySeqDecisions[0] = map[int]paritySeqDecision{
		0: {
			Row:        0,
			Abs:        0,
			Audible:    true,
			NodeType:   model.NodeTypeRegular,
			Missing:    false,
			RecordedAt: time.Unix(0, 0).Add(-250 * time.Millisecond), // past the grace window
		},
	}
	g.parityMu.Unlock()

	g.ClearParityMismatches()
	g.parityScan("test-audio-missing-recent-past")

	found := false
	for _, m := range g.ParityMismatchSnapshot() {
		if m.Kind == "audio_missing" && m.Row == 0 && m.Abs == 0 {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected audio_missing mismatch for recent past beat; got=%v", g.ParityMismatchSnapshot())
	}
}

package ui

import (
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// Regression: parityScan must filter BOTH seq decisions AND audio events by the
// current parityGen, so an entry recorded under a prior structural generation
// (before a live edit bumped parityGen) never participates in a comparison
// against the post-mutation predictor. This is the symmetric generation filter
// the design promises (game_parity_state.go gen-bump contract); without it a
// stale decision recorded just before a graph/length/instrument edit panics the
// app against the new predictor truth.
func TestParityScanFiltersStaleParityGenDecision(t *testing.T) {
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
	g.drum.SetLength(1)
	g.drum.Offset = 0
	g.refreshDrumRow()

	g.nextBeatIdxs = []int{2} // pastExclusive=2 => current beat abs=1
	g.seqNextIdxs = []int{2}

	// Simulate a structural mutation having advanced parityGen AFTER this beat's
	// decision was recorded (the decision carries the prior generation).
	staleGen := g.parityGen.Load()
	g.parityGen.Add(1)

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
			// Not enqueued AND stale generation: under the current generation
			// this would be a genuine audio_missing, but it belongs to a prior
			// structural generation and must be filtered out entirely.
			Enqueued:   false,
			ParityGen:  staleGen,
			RecordedAt: time.Unix(0, 0).Add(-250 * time.Millisecond),
		},
	}
	g.parityMu.Unlock()

	g.ClearParityMismatches()
	g.parityScan("test-stale-paritygen-decision")

	for _, m := range g.ParityMismatchSnapshot() {
		t.Fatalf("stale-parityGen decision must be filtered out, got mismatch: %+v", m)
	}
}

// Counterpart: a decision at the CURRENT parityGen is still compared (the
// filter must not be so broad it stops detecting real issues).
func TestParityScanKeepsCurrentParityGenDecision(t *testing.T) {
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
	g.drum.SetLength(1)
	g.drum.Offset = 0
	g.refreshDrumRow()

	g.nextBeatIdxs = []int{2}
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
			Enqueued:   false, // never enqueued → genuine scheduler bug
			ParityGen:  g.parityGen.Load(),
			RecordedAt: time.Unix(0, 0).Add(-250 * time.Millisecond),
		},
	}
	g.parityMu.Unlock()

	g.ClearParityMismatches()
	g.parityScan("test-current-paritygen-decision")

	found := false
	for _, m := range g.ParityMismatchSnapshot() {
		if m.Kind == "audio_missing" && m.Row == 0 && m.Abs == 0 {
			found = true
		}
	}
	if !found {
		t.Fatalf("current-parityGen genuine audio_missing must still be reported; got=%v", g.ParityMismatchSnapshot())
	}
}

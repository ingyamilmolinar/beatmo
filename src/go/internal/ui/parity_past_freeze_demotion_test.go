package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/timeline"
)

// TestFreezeLoopCommitsAreReleasedNotPlayback locks down the D4 invariant from
// the parity refactor: the safety-net freeze loop in refreshDrumRow must
// commit CommitKindReleased (mutable) when it speculates about a beat the
// audio thread hasn't yet committed. Only applySequencerHighlight is
// authorized to write CommitKindPlayback (the immutable kind).
//
// This test fails before the demotion change and passes after.
func TestFreezeLoopCommitsAreReleasedNotPlayback(t *testing.T) {
	assertDefaultParityState(t)
	g := buildTestGame(t)
	t.Cleanup(g.CloseForTest)

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.start = a
	g.graph.StartNodeID = a.ID
	g.addEdge(a, b)
	g.addEdge(b, a)
	g.updateBeatInfos()

	g.drum.SetLength(8)
	g.drum.Offset = 0

	// Simulate a state where the sequencer has advanced past a few beats but
	// applySequencerHighlight has not run yet. nextBeatIdxs[0] = 4 means
	// rowPastExclusive(0) = 4, so the freeze loop will speculate-commit
	// beats 0..3 to fill the gap.
	g.SetPlaying(true)
	g.nextBeatIdxs = []int{4}
	g.seqNextIdxs = []int{4}
	g.elapsedBeats = 4

	// Refresh: the freeze loop runs and writes commits.
	g.refreshDrumRow()

	// All committed beats in the just-frozen range must be CommitKindReleased,
	// not CommitKindPlayback. Beats committed by applySequencerHighlight (none
	// here, since we never called it) remain absent.
	foundCommit := false
	for abs := 0; abs <= 3; abs++ {
		_, _, kind, ok := g.timelineCommittedWithKind(0, abs)
		if !ok {
			continue
		}
		foundCommit = true
		if kind == timeline.CommitKindPlayback {
			t.Fatalf("beat %d committed as CommitKindPlayback (immutable); want CommitKindReleased — freeze loop must not speculate-commit immutables", abs)
		}
	}
	if !foundCommit {
		t.Fatalf("freeze loop did not commit any beats in the past range; test setup invalid")
	}
}

// TestApplySequencerHighlightStillCommitsPlayback verifies the symmetric
// invariant: applySequencerHighlight is the SOLE source of CommitKindPlayback.
// After a highlight fires, the corresponding beat is committed as immutable
// playback so future structural edits cannot rewrite the audible past.
func TestApplySequencerHighlightStillCommitsPlayback(t *testing.T) {
	assertDefaultParityState(t)
	g := buildTestGame(t)
	t.Cleanup(g.CloseForTest)

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.start = a
	g.graph.StartNodeID = a.ID
	g.addEdge(a, b)
	g.addEdge(b, a)
	g.updateBeatInfos()

	g.drum.SetLength(8)
	g.drum.Offset = 0
	g.SetPlaying(true)

	info := g.beatInfoAtRow(0, 2)
	g.applySequencerHighlight(0, 2, info)

	val, _, kind, ok := g.timelineCommittedWithKind(0, 2)
	if !ok {
		t.Fatalf("expected commit at row=0 abs=2 after applySequencerHighlight")
	}
	if kind != timeline.CommitKindPlayback {
		t.Fatalf("expected CommitKindPlayback at row=0 abs=2, got %v (val=%v)", kind, val)
	}
	// For a regular node the committed value should be the audible truth.
	if !val {
		t.Fatalf("regular-node highlight committed val=false; want true (info.NodeType=%v)", info.NodeType)
	}
}

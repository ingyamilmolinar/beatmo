package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/timeline"
)

// The safety-net freeze speculates about beats the audio thread has not committed, so it writes
// CommitKindReleased; only applySequencerHighlight writes the immutable CommitKindPlayback.
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

	// rowPastExclusive=4 with no highlight applied: the freeze loop fills beats 0..3.
	g.SetPlaying(true)
	g.nextBeatIdxs = []int{4}
	g.seqNextIdxs = []int{4}
	g.elapsedBeats = 4

	// Refresh: the freeze loop runs and writes commits.
	g.refreshDrumRow()

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

// Once committed as Playback, a later structural edit cannot rewrite the audible past.
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

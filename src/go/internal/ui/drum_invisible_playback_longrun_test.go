package ui

import (
	"testing"

	"github.com/ingyamilmolinar/tunkul/core/model"
	"github.com/ingyamilmolinar/tunkul/internal/timeline"
)

// Long-run regression: an invisible playback commit far behind the live head
// must remain immutable even after later graph edits.
func TestDrumView_InvisiblePlaybackCommitStaysFrozenAfterLongRun(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.Layout(1024, 720)

	// Single row with a long non-looping chain (so beyond length we get invisible beats).
	prev := g.tryAddNode(0, 0, model.NodeTypeRegular)
	for i := 1; i <= 220; i++ {
		n := g.tryAddNode(i, 0, model.NodeTypeRegular)
		g.addEdge(prev, n)
		prev = n
	}
	g.start = g.nodeAt(0, 0)
	g.graph.StartNodeID = g.start.ID

	g.drum.SetLength(128)
	g.drum.Offset = 120 // window contains targetAbs
	g.updateBeatInfos()
	g.refreshDrumRow()

	targetAbs := 200 // beyond path length, beatInfo is invisible initially

	// Add a real node at targetAbs and stitch neighbors so predictor wants it ON.
	node := g.tryAddNode(targetAbs, 0, model.NodeTypeRegular)
	if left := g.nodeAt(targetAbs-1, 0); left != nil {
		g.addEdge(left, node)
	}
	if right := g.nodeAt(targetAbs+1, 0); right != nil {
		g.addEdge(node, right)
	}
	g.updateBeatInfos()

	// Simulate long-running playback state after the add: freeze past far ahead.
	g.frozenUpToByRow = []int{targetAbs}
	g.nextBeatIdxs = []int{targetAbs + 40}
	g.elapsedBeats = targetAbs + 50

	// History recorded as playback (immutable) invisible gap at targetAbs.
	g.timelineService().RecordCommitKind(0, targetAbs, false, model.NodeTypeInvisible, timeline.CommitKindPlayback, 256, 256)
	if v, typ, kind, ok := g.timelineCommittedWithKind(0, targetAbs); !ok || kind != timeline.CommitKindPlayback {
		t.Fatalf("failed to seed playback invisible commit: ok=%v val=%v typ=%v kind=%v", ok, v, typ, kind)
	}
	if start, end, ok := g.timelineCommittedRange(0); ok {
		t.Logf("range before refresh: [%d,%d]", start, end)
	}

	if g.frozenUpToByRow[0] != targetAbs {
		t.Fatalf("freeze mutated before refresh: got=%d want=%d", g.frozenUpToByRow[0], targetAbs)
	}

	g.engine.Predictor.Ensure(targetAbs + 1)
	g.refreshDrumRow()
	if start, end, ok := g.timelineCommittedRange(0); ok {
		t.Logf("range after refresh: [%d,%d]", start, end)
	}

	rel := targetAbs - g.drum.Offset
	if rel < 0 || rel >= len(g.drum.Rows[0].Steps) {
		t.Fatalf("targetAbs outside window rel=%d offset=%d len=%d", rel, g.drum.Offset, len(g.drum.Rows[0].Steps))
	}
	got := g.drum.Rows[0].Steps[rel]
	commitVal, typ, kind, ok := g.timelineCommittedWithKind(0, targetAbs)
	if !ok || kind != timeline.CommitKindPlayback {
		t.Fatalf("missing playback commit: ok=%v kind=%v typ=%v", ok, kind, typ)
	}
	if got != commitVal {
		want := g.engine.Predictor.VisibleAt(0, targetAbs)
		t.Fatalf("long-run playback history changed: abs=%d commit=%v got=%v predictor=%v typ=%v kind=%v freeze=%v next=%v",
			targetAbs, commitVal, got, want, typ, kind, g.frozenUpToByRow, g.nextBeatIdxs)
	}
}

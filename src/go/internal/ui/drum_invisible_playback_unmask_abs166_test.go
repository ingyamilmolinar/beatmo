package ui

import (
	"testing"

	"github.com/ingyamilmolinar/tunkul/core/model"
	"github.com/ingyamilmolinar/tunkul/internal/timeline"
)

func TestDrumView_InvisiblePlaybackCommitStaysFrozenAtAbs166(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.Layout(1024, 720)

	// Build long linear chain.
	prev := g.tryAddNode(0, 0, model.NodeTypeRegular)
	for i := 1; i <= 260; i++ {
		n := g.tryAddNode(i, 0, model.NodeTypeRegular)
		g.addEdge(prev, n)
		prev = n
	}
	g.start = g.nodeAt(0, 0)
	g.graph.StartNodeID = g.start.ID
	g.drum.SetLength(128)
	g.drum.Offset = 120 // window that contains abs=166
	g.updateBeatInfos()
	g.refreshDrumRow()

	targetAbs := 166
	// Simulate playback history: frozen past with an invisible playback commit at targetAbs.
	g.frozenUpToByRow = []int{219}
	g.recordTimelineCommitKind(0, targetAbs, false, model.NodeTypeInvisible, timeline.CommitKindPlayback)
	if v, typ, kind, ok := g.timelineCommittedWithKind(0, targetAbs); !ok || kind != timeline.CommitKindPlayback || typ != model.NodeTypeInvisible || v {
		t.Fatalf("expected seeded playback invisible commit at abs=%d: ok=%v val=%v typ=%v kind=%v", targetAbs, ok, v, typ, kind)
	}
	if start, end, ok := g.timelineCommittedRange(0); ok {
		t.Logf("range before update: [%d,%d]", start, end)
	}

	// Pretend playback is ahead of target so it is in the past region.
	g.nextBeatIdxs = []int{216}
	g.elapsedBeats = 220

	// Re-add a real node at targetAbs and refresh paths.
	node := g.tryAddNode(targetAbs, 0, model.NodeTypeRegular)
	if left := g.nodeAt(targetAbs-1, 0); left != nil {
		g.addEdge(left, node)
	}
	if right := g.nodeAt(targetAbs+1, 0); right != nil {
		g.addEdge(node, right)
	}
	g.updateBeatInfos()

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
		t.Fatalf("missing playback commit at abs=%d ok=%v kind=%v typ=%v", targetAbs, ok, kind, typ)
	}
	if got != commitVal {
		want := g.engine.Predictor.VisibleAt(0, targetAbs)
		t.Fatalf("past playback history changed at abs=%d: commit=%v got=%v predictor=%v typ=%v kind=%v freeze=%v next=%v",
			targetAbs, commitVal, got, want, typ, kind, g.frozenUpToByRow, g.nextBeatIdxs)
	}
}

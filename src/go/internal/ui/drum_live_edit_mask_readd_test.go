package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/timeline"
)

// Regression: deleting a node during playback freezes a past entry; re‑adding
// the same node (restoring the path shape) must clear the frozen mask so the
// DrumView window matches the engine predictor.
func TestDrumView_ReaddClearsFrozenMaskEvenIfPathUnchanged(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.Layout(800, 600)
	g.drum.SetLength(64)

	// Build a simple square loop; target node is B (index 1).
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	c := g.tryAddNode(1, 1, model.NodeTypeRegular)
	d := g.tryAddNode(0, 1, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, c)
	g.addEdge(c, d)
	g.addEdge(d, a)
	g.start = a
	g.graph.StartNodeID = a.ID
	g.drum.Rows[0].Origin = a.ID
	g.drum.Rows[0].Node = a

	g.updateBeatInfos()
	g.refreshDrumRow()
	g.drum.SetBPM(120)

	// Start playback and advance a single beat so B becomes the next target.
	pressPlay(t, g.drum)
	_ = g.Update()
	scheduleAbsForMuteTest(g, 0)
	if len(g.nextBeatIdxs) == 0 {
		t.Fatalf("nextBeatIdxs empty")
	}
	targetAbs := g.nextBeatIdxs[0] // next will hit B

	// Delete B during playback; Update once to process delete and freeze.
	if n := g.nodeAt(1, 0); n != nil {
		g.deleteNode(n)
	}
	setPlayStartForAbsFloat(g, float64(targetAbs)-0.2)
	if err := g.Update(); err != nil {
		t.Fatalf("update after delete: %v", err)
	}
	// Re-add B at the same coord, restore edges, path hash likely unchanged.
	b2 := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.deleteEdge(a, d) // break and restore original order
	g.addEdge(a, b2)
	g.addEdge(b2, c)
	g.addEdge(c, d)
	g.addEdge(d, a)
	g.updateBeatInfos()

	// Advance once more; DrumView should now reflect predictor at targetAbs.
	setPlayStartForAbsFloat(g, float64(targetAbs)-0.2)
	if err := g.Update(); err != nil {
		t.Fatalf("update after re-add: %v", err)
	}
	g.refreshDrumRow()

	// Predictor wants B ON at targetAbs.
	g.engine.Predictor.Ensure(targetAbs + 1)
	want := g.engine.Predictor.VisibleAt(0, targetAbs)
	if !want {
		t.Fatalf("predictor expected ON at abs=%d after re-add", targetAbs)
	}
	if targetAbs < g.drum.Offset || targetAbs >= g.drum.Offset+g.drum.Length {
		t.Fatalf("target abs %d not in window [%d,%d)", targetAbs, g.drum.Offset, g.drum.Offset+g.drum.Length)
	}
	j := targetAbs - g.drum.Offset
	got := g.drum.Rows[0].Steps[j]
	if got != want {
		t.Fatalf("DrumView stale after re-add at abs=%d: want=%v got=%v freeze=%v pastMaskCount=%d",
			targetAbs, want, got, g.frozenUpToByRow, len(g.drum.timelinePast[0]))
	}
}

// Repro for the row=4/abs=86 mismatch observed in TIMELINE_TRACE logs:
// remove a node during playback, advance far enough to seed frozen history,
// then re-add the node. A seeded invisible commit before the path-change beat
// should be overridden by the predictor, but the current logic keeps it masked.
func TestDrumView_InvisibleSeedMasksReaddedNode(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.Layout(800, 600)

	// Keep a large window so abs=86 stays inside DrumView.
	g.drum.SetLength(192)
	g.drum.SetBeatLength(g.drum.Length)
	g.graph.SetBeatLength(g.drum.Length)

	// Build a long horizontal chain with a regular node at every subdivision.
	var nodes []*uiNode
	prev := g.tryAddNode(0, 0, model.NodeTypeRegular)
	nodes = append(nodes, prev)
	for i := 1; i <= 130; i++ {
		n := g.tryAddNode(i, 0, model.NodeTypeRegular)
		g.addEdge(prev, n)
		nodes = append(nodes, n)
		prev = n
	}
	g.start = nodes[0]
	g.graph.StartNodeID = nodes[0].ID

	g.updateBeatInfos()
	g.refreshDrumRow()

	targetAbs := 86
	// Simulate playback having advanced past the target so history seeding
	// will cover abs=86. Freeze the past and set elapsed beats accordingly.
	g.SetPlaying(true)
	// Advance far enough that the invisible stale gate is exceeded.
	behind := g.grid.MaxDiv()*2 + 10
	g.elapsedBeats = targetAbs + behind
	g.nextBeatIdxs = []int{g.elapsedBeats}
	g.frozenUpToByRow = []int{g.elapsedBeats + 50}

	// Manually seed a frozen invisible commit before the path-change beat,
	// matching the trace where history seeding masked a re-added node.
	g.recordTimelineCommitKind(0, targetAbs, false, model.NodeTypeInvisible, timeline.CommitKindSeeded)
	g.pathChangeBeatByRow = []int{targetAbs + 10} // change beat after target
	g.refreshDrumRow()

	// Predictor sees the restored node; DrumView stays masked by the frozen commit.
	g.engine.Predictor.Ensure(targetAbs + 1)
	want := g.engine.Predictor.VisibleAt(0, targetAbs)
	if !want {
		v, typ, ok := g.timelineCommitted(0, targetAbs)
		t.Fatalf("sanity: predictor expected ON at abs=%d (commit=%v typ=%v ok=%v freeze=%v changeBeat=%v)",
			targetAbs, v, typ, ok, g.frozenUpToByRow, g.pathChangeBeatByRow[0])
	}
	if targetAbs < g.drum.Offset || targetAbs >= g.drum.Offset+g.drum.Length {
		t.Fatalf("target abs %d not in window [%d,%d)", targetAbs, g.drum.Offset, g.drum.Offset+g.drum.Length)
	}
	got := g.drum.Rows[0].Steps[targetAbs-g.drum.Offset]
	if got != want {
		v, typ, ok := g.timelineCommitted(0, targetAbs)
		t.Fatalf("DrumView masked after re-add at abs=%d: want=%v got=%v commit=%v typ=%v ok=%v freeze=%v changeBeat=%v",
			targetAbs, want, got, v, typ, ok, g.frozenUpToByRow, g.pathChangeBeatByRow[0])
	}
}

// Tail-of-log regression: frozen invisible commit at abs=134 (later 166)
// remains masked because the path-change beat lies ahead and the stale gate
// is large, so DrumView disagrees with the predictor.
func TestDrumView_InvisibleFrozenPastStaysMaskedAfterPathChange(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.Layout(960, 600)

	// Long linear chain to guarantee regular beats through >= 200.
	const chainLen = 240
	prev := g.tryAddNode(0, 0, model.NodeTypeRegular)
	for i := 1; i < chainLen; i++ {
		n := g.tryAddNode(i, 0, model.NodeTypeRegular)
		g.addEdge(prev, n)
		prev = n
	}
	g.start = g.nodeAt(0, 0)
	g.graph.StartNodeID = g.start.ID

	g.drum.SetLength(192)
	g.updateBeatInfos()
	g.refreshDrumRow()

	targetAbs := 134
	// Seed a frozen invisible commit before the path-change beat.
	g.recordTimelineCommitKind(0, targetAbs, false, model.NodeTypeInvisible, timeline.CommitKindSeeded)

	// Simulate playback near the end of the log: path change after target, large freeze.
	g.SetPlaying(true)
	g.elapsedBeats = 198
	g.nextBeatIdxs = []int{198}
	g.frozenUpToByRow = []int{230}
	g.pathChangeBeatByRow = []int{190}

	g.engine.Predictor.Ensure(targetAbs + 1)
	g.refreshDrumRow()

	if targetAbs >= len(g.drum.Rows[0].Steps) {
		t.Fatalf("window too small for target abs=%d len=%d", targetAbs, len(g.drum.Rows[0].Steps))
	}
	want := g.engine.Predictor.VisibleAt(0, targetAbs)
	if !want {
		t.Fatalf("predictor expected ON at abs=%d", targetAbs)
	}
	got := g.drum.Rows[0].Steps[targetAbs]
	if got != want {
		v, typ, ok := g.timelineCommitted(0, targetAbs)
		t.Fatalf("DrumView masked after path change at abs=%d: want=%v got=%v commit=%v typ=%v ok=%v freeze=%v changeBeat=%v",
			targetAbs, want, got, v, typ, ok, g.frozenUpToByRow, g.pathChangeBeatByRow[0])
	}
}

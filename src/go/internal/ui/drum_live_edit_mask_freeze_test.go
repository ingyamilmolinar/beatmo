package ui

import (
	"testing"

	"github.com/ingyamilmolinar/tunkul/core/model"
	"github.com/ingyamilmolinar/tunkul/internal/timeline"
)

// Failing regression: if a frozen past entry marks a future cell off, and the
// graph/path does NOT change (path signature identical), DrumView currently
// keeps the stale mask even though the predictor says the cell is on.
// This mirrors the desktop repro: delete/re-add restores the same path hash
// but the freeze mask remains, so the UI stays silent while audio plays.
func TestDrumView_FrozenMaskBlocksReaddedNode_NoPathChange(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.Layout(800, 600)
	g.drum.SetLength(64)

	// Simple 2-node line to keep path stable.
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, a) // loop
	g.start = a
	g.graph.StartNodeID = a.ID
	g.drum.Rows[0].Origin = a.ID
	g.drum.Rows[0].Node = a

	g.updateBeatInfos()
	g.refreshDrumRow()
	if len(g.nextBeatIdxs) == 0 {
		g.nextBeatIdxs = []int{0}
	}
	// Advance a little so nextBeatIdxs[0]=1 (will hit b).
	pressPlay(t, g.drum)
	_ = g.Update()
	advancePlaybackByAbs(g, 2)
	target := g.nextBeatIdxs[0] // absolute index that will trigger b

	// Simulate a delete that froze the future cell to OFF but the path hash
	// didn't change (we do NOT call updateBeatInfos or alter the graph).
	if len(g.frozenUpToByRow) != len(g.drum.Rows) {
		g.frozenUpToByRow = make([]int, len(g.drum.Rows))
		for i := range g.frozenUpToByRow {
			g.frozenUpToByRow[i] = -1
		}
	}
	g.frozenUpToByRow[0] = target
	g.recordTimelineCommitKind(0, target, false, model.NodeTypeRegular, timeline.CommitKindSeeded)

	// Predictor says b should be visible at target.
	g.engine.Predictor.Ensure(target + 1)
	want := g.engine.Predictor.VisibleAt(0, target)
	if !want {
		t.Fatalf("predictor expected ON at abs=%d", target)
	}

	g.refreshDrumRow()
	if target < g.drum.Offset || target >= g.drum.Offset+g.drum.Length {
		t.Fatalf("target outside window")
	}
	got := g.drum.Rows[0].Steps[target-g.drum.Offset]
	if got != want {
		t.Fatalf("DrumView masked re-added node: abs=%d want=%v got=%v freeze=%v", target, want, got, g.frozenUpToByRow)
	}
}

func TestDrumView_FrozenMaskBeforeNextBeatReleases(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.Layout(800, 600)
	g.drum.SetLength(128)

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, a)
	g.start = a
	g.graph.StartNodeID = a.ID
	g.drum.Rows[0].Origin = a.ID
	g.drum.Rows[0].Node = a

	g.updateBeatInfos()
	g.refreshDrumRow()

	// Simulate runtime state where playback is ahead of the frozen commit.
	target := 134
	g.drum.Offset = 120
	g.nextBeatIdxs = []int{220}
	g.elapsedBeats = g.drum.Offset

	// Freeze the target cell with a seeded commit that disagrees with the
	// predictor so refreshDrumRow must release the stale mask.
	rows := len(g.drum.Rows)
	g.frozenUpToByRow = make([]int, rows)
	for i := range g.frozenUpToByRow {
		g.frozenUpToByRow[i] = -1
	}
	g.frozenUpToByRow[0] = target
	bi := g.beatInfoAtRow(0, target)
	g.engine.Predictor.Ensure(target + 1)
	want := g.engine.Predictor.VisibleAt(0, target)
	if bi.NodeType == model.NodeTypeMute {
		want = g.engine.Predictor.TriggeredAt(0, target)
	}
	g.recordTimelineCommitKind(0, target, !want, bi.NodeType, timeline.CommitKindSeeded)

	if _, _, ok := g.timelineCommitted(0, target); !ok {
		t.Fatalf("expected timeline commit at abs %d", target)
	}

	g.refreshDrumRow()

	if _, _, ok := g.timelineCommitted(0, target); ok {
		t.Fatalf("timeline still has commit after refresh")
	}
	if g.frozenUpToByRow[0] >= target {
		t.Fatalf("freeze not released: %v", g.frozenUpToByRow)
	}
	idx := target - g.drum.Offset
	if idx < 0 || idx >= len(g.drum.Rows[0].Steps) {
		t.Fatalf("target %d outside window offset=%d len=%d", target, g.drum.Offset, len(g.drum.Rows[0].Steps))
	}
	got := g.drum.Rows[0].Steps[idx]
	if got != want {
		t.Fatalf("released cell did not match predictor: abs=%d want=%v got=%v", target, want, got)
	}
}

func TestDrumView_DeleteWhilePlayingKeepsPastHistory(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.Layout(800, 600)
	g.drum.SetLength(64)

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, a)
	g.start = a
	g.graph.StartNodeID = a.ID
	g.drum.Rows[0].Origin = a.ID
	g.drum.Rows[0].Node = a

	g.updateBeatInfos()
	g.refreshDrumRow()

	pressPlay(t, g.drum)
	_ = g.Update()
	advancePlaybackByAbs(g, g.grid.MaxDiv()*2)
	if len(g.nextBeatIdxs) == 0 {
		t.Fatalf("nextBeatIdxs empty after playback")
	}
	pastAbs := g.nextBeatIdxs[0] - 1
	if pastAbs < 0 {
		t.Fatalf("no past index to test")
	}
	info := g.beatInfoAtRow(0, pastAbs)
	if info.NodeID == model.InvalidNodeID {
		t.Fatalf("invalid beat info at abs %d", pastAbs)
	}
	g.recordTimelineCommit(0, pastAbs, true, info.NodeType)
	if len(g.frozenUpToByRow) != len(g.drum.Rows) {
		g.frozenUpToByRow = make([]int, len(g.drum.Rows))
		for i := range g.frozenUpToByRow {
			g.frozenUpToByRow[i] = -1
		}
	}
	g.frozenUpToByRow[0] = pastAbs

	targetNode := g.nodeByID(info.NodeID)
	if targetNode == nil {
		t.Fatalf("node %d missing before delete", info.NodeID)
	}
	g.deleteNode(targetNode)
	g.updateBeatInfos()

	g.refreshDrumRow()

	if _, _, ok := g.timelineCommitted(0, pastAbs); !ok {
		t.Fatalf("timeline commit dropped for past abs=%d", pastAbs)
	}
	rel := pastAbs - g.drum.Offset
	if rel < 0 || rel >= len(g.drum.Rows[0].Steps) {
		t.Fatalf("past abs=%d outside window offset=%d len=%d", pastAbs, g.drum.Offset, len(g.drum.Rows[0].Steps))
	}
	if !g.drum.Rows[0].Steps[rel] {
		t.Fatalf("past step flipped off after delete (abs=%d rel=%d)", pastAbs, rel)
	}
}

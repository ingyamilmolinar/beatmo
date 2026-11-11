package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

func TestTimelineSegmentsTracksPast(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	// Build a simple single-row path.
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.pendingStartRow = -1
	g.start = a
	g.graph.StartNodeID = a.ID

	g.updateBeatInfos()

	if len(g.drum.Rows) == 0 {
		t.Fatalf("no drum rows initialized")
	}

	offset := g.drum.Offset
	g.frozenUpToByRow = []int{offset}
	g.recordTimelineCommit(0, offset, true, model.NodeTypeRegular)

	g.refreshDrumRow()

	segments := g.TimelineSegments(0)
	if segments.Offset != offset {
		t.Fatalf("unexpected offset %d want %d", segments.Offset, offset)
	}
	if len(segments.Past) == 0 {
		t.Fatalf("past segment empty")
	}
	if !segments.Past[0] {
		t.Fatalf("past segment did not capture history state")
	}
	if len(segments.PastTypes) == 0 || segments.PastTypes[0] != model.NodeTypeRegular {
		t.Fatalf("past types missing history node type")
	}
}

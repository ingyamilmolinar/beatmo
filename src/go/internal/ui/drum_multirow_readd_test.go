package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// Regression: deleting and re-adding a node on a non-primary row (e.g., row 4)
// during playback must update the predictor/timeline view immediately so
// DrumView does not show stale off/on states.
func TestMultiRowFutureReaddParity(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.Layout(1024, 720)

	// Ensure 5 rows.
	for len(g.drum.Rows) < 5 {
		g.drum.AddRow()
	}

	buildRow := func(row int, x int) {
		g.pendingStartRow = row
		a := g.tryAddNode(x, 0, model.NodeTypeRegular)
		b := g.tryAddNode(x+1, 0, model.NodeTypeRegular)
		c := g.tryAddNode(x+1, 1, model.NodeTypeRegular)
		d := g.tryAddNode(x, 1, model.NodeTypeRegular)
		g.addEdge(a, b)
		g.addEdge(b, c)
		g.addEdge(c, d)
		g.addEdge(d, a)
		g.pendingStartRow = -1
	}
	// Build disjoint squares; row 4 is the one we will mutate.
	for r := 0; r < 5; r++ {
		buildRow(r, r*3)
	}
	g.updateBeatInfos()
	g.refreshDrumRow()

	// Start playback.
	g.SetPlaying(true)
	for abs := 0; abs <= 8; abs++ {
		scheduleAbsForMuteTest(g, abs)
	}
	g.elapsedBeats = 8
	g.refreshDrumRow()
	if len(g.nextBeatIdxs) < 5 {
		t.Fatalf("expected nextBeatIdxs for 5 rows, got %d", len(g.nextBeatIdxs))
	}

	row := 4
	targetAbs := g.nextBeatIdxs[row] + 1 // future subdivision in row 4
	if targetAbs < 0 {
		targetAbs = g.nextBeatIdxs[row]
	}
	rowLen := len(g.beatInfosByRow[row])
	if rowLen == 0 {
		t.Fatalf("empty beat infos row=%d", row)
	}
	targetAbs = targetAbs % rowLen

	// Delete the node that produces targetAbs.
	targetBi := g.beatInfoAtRow(row, targetAbs)
	if targetBi.NodeID == model.InvalidNodeID {
		t.Fatalf("no node at abs=%d row=%d", targetAbs, row)
	}
	// Record neighbors to rewire later.
	prevBi := g.beatInfoAtRow(row, targetAbs-1)
	nextBi := g.beatInfoAtRow(row, targetAbs+1)
	if node := g.nodeByID(targetBi.NodeID); node != nil {
		g.deleteNode(node)
	} else {
		t.Fatalf("cannot find node id=%d row=%d", targetBi.NodeID, row)
	}
	g.updateBeatInfos()
	g.refreshDrumRow()

	// Re-add the node in the same spot and reclose the loop.
	g.pendingStartRow = row
	newNode := g.tryAddNode(targetBi.I, targetBi.J, model.NodeTypeRegular)
	// Reconnect to a simple line forward/back to guarantee inclusion.
	// Reconnect into the original path shape.
	if p := g.nodeByID(prevBi.NodeID); p != nil {
		g.addEdge(p, newNode)
	}
	if n := g.nodeByID(nextBi.NodeID); n != nil {
		g.addEdge(newNode, n)
	}
	g.pendingStartRow = -1

	g.updateBeatInfos()
	g.engine.Predictor.Ensure(targetAbs + 2)
	g.refreshDrumRow()

	idx := targetAbs - g.drum.Offset
	if idx < 0 || idx >= len(g.drum.Rows[row].Steps) {
		t.Fatalf("target idx %d out of window len=%d offset=%d", idx, len(g.drum.Rows[row].Steps), g.drum.Offset)
	}
	if !g.drum.Rows[row].Steps[idx] {
		t.Fatalf("expected step true after re-add at abs=%d row=%d (idx=%d) steps=%v", targetAbs, row, idx, g.drum.Rows[row].Steps)
	}
}

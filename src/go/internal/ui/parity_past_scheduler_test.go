package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// Scheduler parity should ignore beats that are already behind the playhead.
func TestParityCheckIgnoresPastBeats(t *testing.T) {
	assertDefaultParityState(t)
	prevFatal := parityFatalEnabled.Load()
	SetParityFatal(true)
	t.Cleanup(func() { SetParityFatal(prevFatal) })

	g := buildTestGame(t)
	g.parityWatch = parityWatchPanic
	t.Cleanup(g.CloseForTest)

	root := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n1 := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.graph.StartNodeID = root.ID
	g.start = root
	g.drum.Rows[0].Origin = root.ID
	g.drum.Rows[0].Node = root
	g.addEdge(root, n1)
	g.addEdge(n1, root)
	g.updateBeatInfos()
	g.drum.SetLength(8)
	g.drum.Offset = 0
	g.refreshDrumRow()

	// Simple slate with hits at 0, 2, and 4 so we can trigger/parity check.
	g.drum.Rows[0].Steps = []bool{true, false, true, false, true, false, false, false}

	// Pretend playhead next index is 5 (last scheduled = 4). Indices <4 are past.
	g.nextBeatIdxs = []int{5}

	// Past beats (< playheadFloor-1) should not panic even when slate differs.
	g.parityCheck(0, 0, g.beatInfoAtRow(0, 0), false, "test_past", false) // 0 < 4
	g.parityCheck(0, 1, g.beatInfoAtRow(0, 1), false, "test_past", false) // 1 < 4

	// Current-ish beat (just behind nextBeatIdxs) should still panic on mismatch.
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic on current-beat mismatch")
		}
	}()
	g.parityCheck(0, 4, g.beatInfoAtRow(0, 4), false, "test_current", false) // 4 == playheadFloor()-1
}

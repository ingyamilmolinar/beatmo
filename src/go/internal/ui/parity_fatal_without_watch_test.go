package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// Even when parityWatch is off, PARITY_FATAL should still trigger panics on mismatches.
func TestParityFatalTriggersWithoutWatch(t *testing.T) {
	assertDefaultParityState(t)
	prevFatal := parityFatalEnabled.Load()
	SetParityFatal(true)
	t.Cleanup(func() { SetParityFatal(prevFatal) })
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.parityWatch = parityWatchOff
	g.Layout(640, 480)

	// Prepare a minimal playing state with a single audible cell.
	g.drum.SetLength(1)
	root := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n1 := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.graph.StartNodeID = root.ID
	g.start = root
	g.drum.Rows[0].Origin = root.ID
	g.drum.Rows[0].Node = root
	g.addEdge(root, n1)
	g.addEdge(n1, root)
	g.updateBeatInfos()

	abs := 0
	if len(g.nextBeatIdxs) > 0 {
		abs = g.nextBeatIdxs[0]
	}
	if abs < 0 {
		abs = 0
	}

	// Force an on-screen mismatch at the first non-past cell so parityScan must fatal.
	g.drum.SetLength(1)
	g.drum.Offset = abs
	g.refreshDrumRow()
	g.drum.Rows[0].Steps = []bool{false} // view says off
	g.SetPlaying(true)

	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected parity fatal panic when parityWatch off but PARITY_FATAL enabled")
		}
	}()

	g.parityScan("test-fatal-no-watch")
}

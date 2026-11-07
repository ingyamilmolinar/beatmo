package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// A "refresh" scan visits only abs congruent to the rotating phase mod stride, so a persistent mismatch must
// surface within one full cycle rather than be skipped forever.
func TestParityStrideEventuallyFiresMismatch(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	g.parityWatch = parityWatchLog
	g.ClearParityMismatches()

	// A self-loop makes every abs visible.
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.start = a
	g.graph.StartNodeID = a.ID
	g.drum.Rows[0].Origin = a.ID
	g.drum.Rows[0].Node = a
	g.addEdge(a, a)
	g.updateBeatInfos()
	ensureRowInstrumentsAvailable(t, g)
	g.drum.SetLength(16)
	g.drum.Offset = 0
	g.refreshDrumRow()

	if len(g.drum.Rows) == 0 || len(g.drum.Rows[0].Steps) < 8 {
		t.Fatalf("unexpected empty/short drum view state: %+v", g.drum.Rows)
	}
	if !g.drum.Rows[0].Steps[7] {
		t.Fatalf("expected step 7 to be visible pre-corruption")
	}

	// Only a "refresh" scan uses the stride, and it bails out when not playing.
	g.SetPlaying(true)

	const stride = 3
	g.parityScanStride = stride
	g.parityScanPhase = 0

	const plantedRow, plantedAbs = 0, 7
	// parityScan never mutates the slate; re-asserted each pass in case a scan path ever rebuilds the row.
	g.drum.Rows[plantedRow].Steps[plantedAbs] = false

	found := false
	var lastSnapshot []mismatchEntry
	for i := 0; i < stride*2+2; i++ {
		g.drum.Rows[plantedRow].Steps[plantedAbs] = false
		g.ClearParityMismatches()
		g.parityScan("refresh")
		lastSnapshot = g.ParityMismatchSnapshot()
		for _, m := range lastSnapshot {
			if m.Kind == "view_vs_truth" && m.Row == plantedRow && m.Abs == plantedAbs {
				found = true
			}
		}
		if found {
			break
		}
	}
	if !found {
		t.Fatalf("stride %d never scanned mismatching abs=%d within %d scans; last snapshot=%+v",
			stride, plantedAbs, stride*2+2, lastSnapshot)
	}
}

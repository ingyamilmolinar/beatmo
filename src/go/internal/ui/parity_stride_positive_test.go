package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// TestParityStrideEventuallyFiresMismatch guards against a stride bug that
// permanently skips a mismatching abs when parityScanStride > 1 rotates the
// row-scan phase across successive "refresh" scans. A single parityScan call
// with reason=="refresh" only visits abs values congruent to the current
// phase mod stride (game_parity_diff_scan.go:356-371); parityScanPhase
// auto-rotates by one on every such call (game_parity_diff_scan.go:187-189).
// A persistent mismatch planted at a single abs must therefore surface
// within one full stride cycle, not be skipped forever.
func TestParityStrideEventuallyFiresMismatch(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	g.parityWatch.Store(parityWatchLog)
	g.ClearParityMismatches()

	// Self-looping single node so the predictor considers every abs visible
	// (mirrors the setup in parity_cached_view_test.go).
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

	// parityScan bails out early on "refresh" when not playing
	// (game_parity_diff_scan.go:176-179), and reason=="refresh" is required
	// to activate the stride/phase logic at all (stride is forced to 1 for
	// any other reason string).
	g.SetPlaying(true)

	const stride = 3
	g.parityScanStride = stride
	g.parityScanPhase = 0

	const plantedRow, plantedAbs = 0, 7
	// Plant a persistent mismatch: predictor says visible, slate disagrees.
	// parityScan never mutates the slate, so this stays corrupted across
	// every iteration below; re-assert it defensively each pass in case a
	// future refactor makes some scan path rebuild the row in between.
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

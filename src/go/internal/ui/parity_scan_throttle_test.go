package ui

import (
	"testing"

	"github.com/ingyamilmolinar/tunkul/core/model"
)

func buildParityMismatchGame(t *testing.T) *Game {
	t.Helper()
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	g.parityWatch = parityWatchLog

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.start = a
	g.graph.StartNodeID = a.ID
	g.drum.Rows[0].Origin = a.ID
	g.drum.Rows[0].Node = a
	g.addEdge(a, a)
	g.updateBeatInfos()
	g.drum.SetLength(1)
	g.drum.Offset = 0
	g.refreshDrumRow()
	if len(g.drum.Rows) == 0 || len(g.drum.Rows[0].Steps) == 0 {
		t.Fatalf("unexpected empty drum view state")
	}
	// Force a stale slate: predictor expects on at abs=0, slate says off.
	g.drum.Rows[0].Steps[0] = false
	g.nextBeatIdxs = []int{0}
	g.seqNextIdxs = []int{0}
	g.SetPlaying(true)
	g.elapsedBeats = -1
	g.ClearParityMismatches()
	return g
}

func TestParityScanThrottleSkipsRefreshUntilDue(t *testing.T) {
	g := buildParityMismatchGame(t)
	g.parityScanEvery = 3
	g.parityScanStride = 1
	g.parityScanLastFrame = g.frame

	g.parityScan("refresh")
	if got := len(g.ParityMismatchSnapshot()); got != 0 {
		t.Fatalf("expected no mismatches while throttled, got %d", got)
	}

	g.frame += 3
	g.parityScan("refresh")
	if got := len(g.ParityMismatchSnapshot()); got == 0 {
		t.Fatalf("expected mismatches once scan is due")
	}
}

func TestParityScanExplicitBypassesThrottle(t *testing.T) {
	g := buildParityMismatchGame(t)
	g.parityScanEvery = 10
	g.parityScanStride = 1
	g.parityScanLastFrame = g.frame

	g.parityScan("test-explicit")
	if got := len(g.ParityMismatchSnapshot()); got == 0 {
		t.Fatalf("expected explicit parity scan to run despite throttle")
	}
}

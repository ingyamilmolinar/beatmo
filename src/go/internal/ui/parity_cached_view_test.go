//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

func TestParityScanUsesCachedViewState(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	g.parityWatch = parityWatchLog
	g.ClearParityMismatches()

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.start = a
	g.graph.StartNodeID = a.ID
	g.drum.Rows[0].Origin = a.ID
	g.drum.Rows[0].Node = a
	g.addEdge(a, a)
	g.updateBeatInfos()
	ensureRowInstrumentsAvailable(t, g)
	g.drum.SetLength(1)
	g.drum.Offset = 0
	g.refreshDrumRow()

	if len(g.drum.Rows) == 0 || len(g.drum.Rows[0].Steps) == 0 {
		t.Fatalf("unexpected empty drum view state")
	}
	if !g.drum.Rows[0].Steps[0] {
		t.Fatalf("expected initial step to be on")
	}

	g.drum.ensureRowCache()
	g.drum.buildRowSprite(0)
	if len(g.drum.rowCache) == 0 || g.drum.rowCache[0] == nil {
		t.Fatalf("expected row cache to be built")
	}

	// Disable the node so predictor/view should turn the step off, but keep the cache stale.
	g.graph.SetNodeLogic(a.ID, func(ctx model.NodeContext) model.NodeDecision {
		disabled := false
		return model.NodeDecision{Enabled: &disabled}
	})
	g.updateBeatInfos()
	g.refreshDrumRow()

	if g.drum.Rows[0].Steps[0] {
		t.Fatalf("expected step to turn off after logic disable")
	}
	if len(g.drum.rowDirty) > 0 {
		g.drum.rowDirty[0] = false
	}
	if len(g.drum.rowFullDirty) > 0 {
		g.drum.rowFullDirty[0] = false
	}
	if v, ok := g.drum.cachedRowState(0, 0); !ok {
		t.Fatalf("expected cached row state to be readable (rowDirty=%v rowFullDirty=%v cacheW=%d cacheH=%d cacheLen=%d cacheOff=%v)",
			g.drum.rowDirty, g.drum.rowFullDirty, g.drum.rowCacheW, g.drum.rowCacheH, g.drum.rowCacheLen, g.drum.rowCacheOff)
	} else if !v {
		t.Fatalf("expected cached row state to remain on")
	}
	g.nextBeatIdxs = []int{0}
	g.seqNextIdxs = []int{0}

	g.ClearParityMismatches()
	g.parityScan("test-cache-view")

	found := false
	for _, m := range g.ParityMismatchSnapshot() {
		if m.Kind == "view_vs_truth" && m.Abs == 0 {
			found = true
			if m.Actual == g.drum.Rows[0].Steps[0] {
				t.Fatalf("expected cached view mismatch; got entry=%+v steps=%v", m, g.drum.Rows[0].Steps[0])
			}
		}
	}
	if !found {
		t.Fatalf("expected view_vs_truth mismatch using cached state, got=%v", g.ParityMismatchSnapshot())
	}
}

func TestParityScanCachedViewRebuildClearsMismatch(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	g.parityWatch = parityWatchLog
	g.ClearParityMismatches()

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.start = a
	g.graph.StartNodeID = a.ID
	g.drum.Rows[0].Origin = a.ID
	g.drum.Rows[0].Node = a
	g.addEdge(a, a)
	g.updateBeatInfos()
	ensureRowInstrumentsAvailable(t, g)
	g.drum.SetLength(1)
	g.drum.Offset = 0
	g.refreshDrumRow()

	g.drum.ensureRowCache()
	g.drum.buildRowSprite(0)

	g.graph.SetNodeLogic(a.ID, func(ctx model.NodeContext) model.NodeDecision {
		disabled := false
		return model.NodeDecision{Enabled: &disabled}
	})
	g.updateBeatInfos()
	g.refreshDrumRow()

	// Rebuild row cache to match the updated slate.
	if len(g.drum.rowFullDirty) > 0 {
		g.drum.rowFullDirty[0] = true
	}
	g.drum.buildRowSprite(0)

	g.nextBeatIdxs = []int{0}
	g.seqNextIdxs = []int{0}

	g.ClearParityMismatches()
	g.parityScan("test-cache-rebuild")

	for _, m := range g.ParityMismatchSnapshot() {
		if m.Kind == "view_vs_truth" {
			t.Fatalf("unexpected view_vs_truth mismatch after cache rebuild: %+v", m)
		}
	}
}

func TestParityScanDetectsSeqVsViewMismatch(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	g.parityWatch = parityWatchLog
	g.ClearParityMismatches()

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
	if len(g.drum.Rows) == 0 || len(g.drum.Rows[0].Steps) == 0 || !g.drum.Rows[0].Steps[0] {
		t.Fatalf("expected initial step to be on")
	}

	// Record a decision. Since the node is visible, recordSeqDecision will
	// query the predictor and set Visible=true.
	g.recordSeqDecision(0, 0, true, model.NodeTypeRegular, false)
	g.nextBeatIdxs = []int{0}
	g.seqNextIdxs = []int{0}

	// Manually corrupt the slate to create a mismatch: predictor says
	// VisibleAt=true, but slate now shows false.
	g.drum.Rows[0].Steps[0] = false

	g.ClearParityMismatches()
	g.parityScan("test-seq-vs-view")

	// The parity scan has multiple checks that compare view truth against the
	// slate. Either view_vs_truth or seq_vs_view can fire when the slate is
	// corrupted. Both detect the same class of mismatches (predictor visible
	// state vs DrumView slate).
	found := false
	for _, m := range g.ParityMismatchSnapshot() {
		if (m.Kind == "seq_vs_view" || m.Kind == "view_vs_truth") && m.Abs == 0 {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected seq_vs_view or view_vs_truth mismatch, got=%v", g.ParityMismatchSnapshot())
	}
}

func TestParityScanSeqVsViewAligned(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	g.parityWatch = parityWatchLog
	g.ClearParityMismatches()

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
	if len(g.drum.Rows) == 0 || len(g.drum.Rows[0].Steps) == 0 || !g.drum.Rows[0].Steps[0] {
		t.Fatalf("expected initial step to be on")
	}

	g.recordSeqDecision(0, 0, true, model.NodeTypeRegular, false)
	g.nextBeatIdxs = []int{0}
	g.seqNextIdxs = []int{0}

	g.ClearParityMismatches()
	g.parityScan("test-seq-vs-view-aligned")

	for _, m := range g.ParityMismatchSnapshot() {
		if m.Kind == "seq_vs_view" {
			t.Fatalf("unexpected seq_vs_view mismatch when aligned: %+v", m)
		}
	}
}

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// TestParityDetectsViewVsTruthForMuteNode is the mute-node true-positive
// counterpart to TestParityScanUsesCachedViewState (parity_cached_view_test.go),
// which only exercises the NodeTypeRegular branch of the view_vs_truth check
// (game_parity_diff_scan.go ~L379-386: for a NodeTypeRegular cell the
// predictor truth is `pred.VisibleAt`, but for a NodeTypeMute cell it's
// `pred.TriggeredAt` instead). That branch previously had no positive
// coverage. This test builds a genuine mute node whose predictor truth
// (TriggeredAt) is true, corrupts the rendered slate to the opposite value,
// and asserts view_vs_truth fires for that cell with NodeType==NodeTypeMute
// and Expected==the TriggeredAt value -- proving the TriggeredAt branch (not
// VisibleAt) is what the comparator used.
func TestParityDetectsViewVsTruthForMuteNode(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	g.parityWatch.Store(parityWatchLog)
	g.ClearParityMismatches()

	// Two-node loop: start (regular) -> mute -> start. Mirrors the
	// construction used by TestApplySequencerHighlight_CommitsMuteAsOn
	// (mute_timeline_commit_test.go) and TestMuteSequencerMatchesPredictorLogicCallback
	// (mute_logic_unified_test.go). No custom node logic is attached to the
	// mute node, so per TestApplySequencerHighlight_CommitsMuteAsOn it
	// defaults to TriggeredAt==true every time it's reached; we assert that
	// explicitly below rather than assuming it.
	start := g.tryAddNode(0, 0, model.NodeTypeRegular)
	mute := g.tryAddNode(1, 0, model.NodeTypeMute)
	if start == nil || mute == nil {
		t.Fatalf("failed to create start/mute nodes")
	}
	g.addEdge(start, mute)
	g.addEdge(mute, start)
	g.start = start
	g.graph.StartNodeID = start.ID
	g.drum.Rows[0].Origin = start.ID
	g.drum.Rows[0].Node = start
	g.updateBeatInfos()
	ensureRowInstrumentsAvailable(t, g)

	if len(g.beatInfosByRow) == 0 || len(g.beatInfosByRow[0]) == 0 {
		t.Fatalf("beat infos not generated")
	}
	muteAbs := -1
	for i, bi := range g.beatInfosByRow[0] {
		if bi.NodeType == model.NodeTypeMute {
			muteAbs = i
			break
		}
	}
	if muteAbs < 0 {
		t.Fatalf("no mute beat found in beat infos: %+v", g.beatInfosByRow[0])
	}

	// Render a window comfortably covering the mute beat.
	g.drum.SetLength(muteAbs + 2)
	g.drum.Offset = 0
	g.refreshDrumRow()

	if len(g.drum.Rows) == 0 || len(g.drum.Rows[0].Steps) <= muteAbs {
		t.Fatalf("unexpected short drum view state: steps=%d muteAbs=%d", len(g.drum.Rows[0].Steps), muteAbs)
	}
	if len(g.drum.Rows[0].CellTypes) <= muteAbs || g.drum.Rows[0].CellTypes[muteAbs] != model.NodeTypeMute {
		t.Fatalf("expected CellTypes[%d]=Mute, got %+v", muteAbs, g.drum.Rows[0].CellTypes)
	}

	// Confirm the predictor truth this branch reads is TriggeredAt (not
	// VisibleAt) and that it's true, so corrupting the slate to false is a
	// genuine mismatch rather than a vacuous no-op.
	g.engine.Predictor.Ensure(muteAbs + 2)
	truth := g.engine.Predictor.TriggeredAt(0, muteAbs)
	if !truth {
		t.Fatalf("expected predictor TriggeredAt(0,%d)=true for an unlogic-gated mute node, got false", muteAbs)
	}

	// Corrupt the rendered slate to the opposite of predictor truth. No row
	// cache is built here, so parityScan's slateAt() reads g.drum.Rows[0].Steps
	// directly (mirrors TestParityScanDetectsSeqVsViewMismatch).
	g.drum.Rows[0].Steps[muteAbs] = !truth

	g.nextBeatIdxs = []int{0}
	g.seqNextIdxs = []int{0}

	g.ClearParityMismatches()
	g.state.SetPaused(true) // production scans a paused transport, never a stopped one
	g.parityScan("test-mute-view-vs-truth")

	found := false
	for _, m := range g.ParityMismatchSnapshot() {
		if m.Kind == "view_vs_truth" && m.Row == 0 && m.Abs == muteAbs {
			found = true
			if m.NodeType != model.NodeTypeMute {
				t.Fatalf("expected mismatch NodeType=Mute, got %v (entry=%+v)", m.NodeType, m)
			}
			if m.Expected != truth {
				t.Fatalf("expected mismatch Expected to equal predictor TriggeredAt truth=%v, got=%v (entry=%+v)", truth, m.Expected, m)
			}
			if m.Actual != g.drum.Rows[0].Steps[muteAbs] {
				t.Fatalf("expected mismatch Actual to equal corrupted slate value; entry=%+v", m)
			}
		}
	}
	if !found {
		t.Fatalf("expected view_vs_truth mismatch for mute node at abs=%d, got=%v", muteAbs, g.ParityMismatchSnapshot())
	}
}

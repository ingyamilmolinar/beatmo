package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/timeline"
)

func TestReconcileMutableTimeline_RewritesSeededButNotReleased(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.drum.Rows = []*DrumRow{{}}
	g.drum.SetLength(8)
	g.drum.Offset = 0
	g.beatInfosByRow = [][]model.BeatInfo{make([]model.BeatInfo, 8)}
	for i := range g.beatInfosByRow[0] {
		g.beatInfosByRow[0][i] = model.BeatInfo{NodeID: model.NodeID(i + 1), NodeType: model.NodeTypeRegular}
	}
	g.isLoopByRow = []bool{false}

	// Seed a stale mutable commit that should reconcile to predictor truth.
	g.recordTimelineCommitKind(0, 3, false, model.NodeTypeRegular, timeline.CommitKindSeeded)
	// A released commit should be treated as bookkeeping and not be rewritten.
	g.recordTimelineCommitKind(0, 4, false, model.NodeTypeRegular, timeline.CommitKindReleased)

	predictAt := func(abs int, bi model.BeatInfo) bool {
		_ = bi
		return abs == 3 || abs == 4
	}
	g.reconcileMutableTimeline(0, 0, 8, predictAt)

	// Seeded -> Released with predictor value.
	val, typ, kind, ok := g.timelineCommittedWithKind(0, 3)
	if !ok || !val || typ != model.NodeTypeRegular || kind != timeline.CommitKindReleased {
		t.Fatalf("seeded commit not reconciled: ok=%v val=%v typ=%v kind=%v", ok, val, typ, kind)
	}

	// Released stays untouched (value preserved for debugging).
	val, typ, kind, ok = g.timelineCommittedWithKind(0, 4)
	if !ok || val || typ != model.NodeTypeRegular || kind != timeline.CommitKindReleased {
		t.Fatalf("released commit should remain unchanged: ok=%v val=%v typ=%v kind=%v", ok, val, typ, kind)
	}
}

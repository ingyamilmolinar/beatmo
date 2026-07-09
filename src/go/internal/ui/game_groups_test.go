package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// buildGroupLoopCircuit creates a 2-node loop (a -> b -> a) on row 0 and
// returns the game plus node IDs. Mirrors the circuit-building pattern used
// by probability_prediction_consistency_test.go's buildProbChain (tryAddNode
// + addEdge + explicit g.start/g.graph.StartNodeID + updateBeatInfos), layered
// on this package's canonical functional-test constructor (see
// undo_seam_test.go:42-56 / testutil_test.go header).
func buildGroupLoopCircuit(t *testing.T) (*Game, model.NodeID, model.NodeID) {
	t.Helper()
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)
	g.updateBeatInfos()

	na := g.tryAddNode(0, 0, model.NodeTypeRegular)
	nb := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(na, nb)
	g.addEdge(nb, na)
	g.start = na
	g.graph.StartNodeID = na.ID
	g.updateBeatInfos()
	return g, na.ID, nb.ID
}

func TestGroupRulePitchAppliesByRound(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	gid, err := g.graph.CreateGroup("", []model.NodeID{aID, bID})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	if err := g.graph.SetGroupRules(gid, []model.GroupRule{
		{Param: model.GroupParamPitch, Delta: 2, EveryN: 1},
	}); err != nil {
		t.Fatalf("SetGroupRules: %v", err)
	}

	loopStart := g.loopStartByRow[0]
	loopLen := len(g.beatInfosByRow[0]) - loopStart
	if !g.isLoopByRow[0] || loopLen <= 0 {
		t.Fatalf("expected loop row, isLoop=%v loopLen=%d", g.isLoopByRow[0], loopLen)
	}

	infoAt := func(idx int) model.BeatInfo { return g.beatInfoAtRow(0, idx) }

	// Round 0: base pitch (0).
	_, p0, _ := g.evalNodeParamsOnly(0, loopStart, infoAt(loopStart))
	if p0 != 0 {
		t.Fatalf("round 0 pitch = %v, want 0", p0)
	}
	// Round 2: +4 semitones (delta 2 x 2 rounds).
	idx2 := loopStart + 2*loopLen
	_, p2, _ := g.evalNodeParamsOnly(0, idx2, infoAt(idx2))
	if p2 != 4 {
		t.Fatalf("round 2 pitch = %v, want 4", p2)
	}
	// Step boundary: pitch changes exactly AT the round's loopStart, and the
	// last index of the previous round still has the previous value.
	_, pPrev, _ := g.evalNodeParamsOnly(0, idx2-1, infoAt(idx2-1))
	if pPrev != 2 {
		t.Fatalf("last idx of round 1 pitch = %v, want 2", pPrev)
	}
	// Clamp: far future rounds clamp at +24.
	idxFar := loopStart + 200*loopLen
	_, pFar, _ := g.evalNodeParamsOnly(0, idxFar, infoAt(idxFar))
	if pFar != 24 {
		t.Fatalf("far round pitch = %v, want clamped 24", pFar)
	}
}

func TestGroupRuleInertOnUngroupedNodeAndNonLoop(t *testing.T) {
	g, aID, _ := buildGroupLoopCircuit(t)
	// No group at all -> identity behavior (existing semantics untouched).
	info := g.beatInfoAtRow(0, 0)
	_, p, _ := g.evalNodeParamsOnly(0, 100, info)
	if p != 0 {
		t.Fatalf("ungrouped pitch = %v, want 0", p)
	}
	// Group with rule on a but round always 0 when row isn't a loop:
	// covered at model level by TestGroupRoundMath; here assert a group
	// WITHOUT rules changes nothing even for members.
	gid, _ := g.graph.CreateGroup("", []model.NodeID{aID})
	_ = gid
	_, p, _ = g.evalNodeParamsOnly(0, 100, info)
	pitchAtRound2 := p
	if pitchAtRound2 != 0 {
		t.Fatalf("rule-less group must be inert, pitch = %v", p)
	}
}

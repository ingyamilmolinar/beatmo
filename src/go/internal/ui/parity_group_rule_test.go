package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// A group rule steps vol/pitch/dur across rounds and must never touch whether a node triggers or is audible.
func TestParityGroupRuleNeverAltersTriggerParity(t *testing.T) {
	assertDefaultParityState(t)
	g, aID, bID := buildGroupLoopCircuit(t)

	for i := range g.drum.Rows[0].Steps {
		g.drum.Rows[0].Steps[i] = true
	}
	g.refreshDrumRow()

	// Baseline (no group yet): predictor/audible agree with the rendered Steps.
	baseline := append([]bool(nil), g.drum.Rows[0].Steps...)
	for i, on := range baseline {
		abs := g.drum.Offset + i
		if want := g.engine.Predictor.VisibleAt(0, abs); want != on {
			t.Fatalf("baseline predictor mismatch at abs=%d: got=%v want=%v", abs, want, on)
		}
	}

	gid, err := g.graph.CreateGroup("", []model.NodeID{aID, bID})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	if err := g.graph.SetGroupRules(gid, []model.GroupRule{
		{Param: model.GroupParamPitch, Delta: 2, EveryN: 1},
	}); err != nil {
		t.Fatalf("SetGroupRules: %v", err)
	}
	if err := g.graph.SetGroupRulesEnabled(gid, true); err != nil {
		t.Fatalf("SetGroupRulesEnabled: %v", err)
	}
	g.refreshDrumRow()

	for i, on := range baseline {
		abs := g.drum.Offset + i
		if got := g.drum.Rows[0].Steps[i]; got != on {
			t.Fatalf("group rule changed rendered Steps at i=%d: got=%v want=%v", i, got, on)
		}
		if got := g.engine.Predictor.VisibleAt(0, abs); got != on {
			t.Fatalf("group rule changed predictor VisibleAt at abs=%d: got=%v want=%v", abs, got, on)
		}
	}

	// nodeTriggered in the (row, idx, info) form both evalNodeParamsOnly callers use must still fire at the round
	// boundary.
	loopStart := g.loopStartByRow[0]
	loopLen := len(g.beatInfosByRow[0]) - loopStart
	if !g.isLoopByRow[0] || loopLen <= 0 {
		t.Fatalf("expected loop row, isLoop=%v loopLen=%d", g.isLoopByRow[0], loopLen)
	}
	infoAt := func(idx int) model.BeatInfo { return g.beatInfoAtRow(0, idx) }
	for round := 0; round < 3; round++ {
		idx := loopStart + round*loopLen
		info := infoAt(idx)
		_, pitch, _ := g.evalNodeParamsOnly(0, idx, info)
		want := float64(round) * 2
		if pitch != want {
			t.Fatalf("round %d pitch = %v, want %v", round, pitch, want)
		}
		if !g.nodeTriggered(0, idx, info) {
			t.Fatalf("round %d: node must still trigger despite the pitch rule", round)
		}
		if want := g.engine.Predictor.VisibleAt(0, idx); !want {
			t.Fatalf("round %d: predictor must still show the node visible/firing despite the pitch rule", round)
		}
	}
}

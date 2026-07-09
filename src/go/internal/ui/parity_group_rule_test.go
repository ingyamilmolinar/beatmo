package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// TestParityGroupRuleNeverAltersTriggerParity is the spec §8 gap: a group
// rule (batch pitch/volume/duration stepping across rounds) must only ever
// touch vol/pitch/dur — never a node's trigger/audible decision. This drives
// the same predictor-vs-audible comparison TestMuteNodeDrumViewPredictionConsistency
// (mute_node_audio_test.go) uses, applied to a 2-node loop group with a
// pitch rule (delta 2, every 1 round), plus the evalNodeParamsOnly
// round-boundary check from TestGroupRulePitchAppliesByRound
// (game_groups_test.go) strengthened to also assert trigger parity at the
// exact (row, idx, info) form both callers (game_tick_highlight.go and
// game_sequencer_schedule.go) use.
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
	g.refreshDrumRow()

	// Trigger parity must be UNCHANGED by the rule: same rendered Steps, same
	// predictor VisibleAt, at every position the baseline covered.
	for i, on := range baseline {
		abs := g.drum.Offset + i
		if got := g.drum.Rows[0].Steps[i]; got != on {
			t.Fatalf("group rule changed rendered Steps at i=%d: got=%v want=%v", i, got, on)
		}
		if got := g.engine.Predictor.VisibleAt(0, abs); got != on {
			t.Fatalf("group rule changed predictor VisibleAt at abs=%d: got=%v want=%v", abs, got, on)
		}
	}

	// Round-boundary pitch stepping (game_groups_test.go's
	// TestGroupRulePitchAppliesByRound), strengthened here to also assert
	// nodeTriggered (the (row, idx, info) form both game_tick_highlight.go's
	// and game_sequencer_schedule.go's evalNodeParamsOnly callers use)
	// agrees the node still fires despite the rule.
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

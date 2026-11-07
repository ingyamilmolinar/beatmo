package model

import "testing"

func idxFor(groups ...*NodeGroup) *GroupIndex {
	m := map[GroupID]*NodeGroup{}
	for _, g := range groups {
		m[g.ID] = g
	}
	return BuildGroupIndex(m)
}

func mustRule(t *testing.T, r GroupRule) GroupRule {
	t.Helper()
	norm, err := normalizeGroupRules([]GroupRule{r})
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	return norm[0]
}

func TestGroupRoundMath(t *testing.T) {
	cases := []struct{ idx, start, length, want int }{
		{0, 0, 4, 0}, {3, 0, 4, 0}, {4, 0, 4, 1}, {11, 0, 4, 2},
		{2, 3, 4, 0},  // before loop start → round 0
		{7, 3, 4, 1},  // (7-3)/4 = 1
		{5, 0, 0, 0},  // no loop (len 0) → 0
		{5, 0, -1, 0}, // defensive
	}
	for _, c := range cases {
		if got := GroupRound(c.idx, c.start, c.length); got != c.want {
			t.Errorf("GroupRound(%d,%d,%d) = %d, want %d", c.idx, c.start, c.length, got, c.want)
		}
	}
}

func TestGroupEffectSingleRule(t *testing.T) {
	r := mustRule(t, GroupRule{Param: GroupParamPitch, Delta: 2, EveryN: 4})
	idx := idxFor(&NodeGroup{ID: 1, NodeIDs: []NodeID{7}, Rules: []GroupRule{r}, RulesEnabled: true})
	for _, c := range []struct {
		round int
		want  float64
	}{{0, 0}, {3, 0}, {4, 2}, {8, 4}, {100, 24} /* clamped to default max 24 */} {
		pd, vm, dm := GroupEffect(idx, 7, c.round, 0, 1)
		if pd != c.want || vm != 1 || dm != 1 {
			t.Errorf("round %d: got (%v,%v,%v), want (%v,1,1)", c.round, pd, vm, dm, c.want)
		}
	}
	// Unruled node and nil index are identity.
	if pd, vm, dm := GroupEffect(idx, 99, 8, 0, 1); pd != 0 || vm != 1 || dm != 1 {
		t.Errorf("unruled node: got (%v,%v,%v)", pd, vm, dm)
	}
	if pd, vm, dm := GroupEffect(nil, 7, 8, 0, 1); pd != 0 || vm != 1 || dm != 1 {
		t.Errorf("nil index: got (%v,%v,%v)", pd, vm, dm)
	}
}

func TestGroupEffectVolumeDurationRamp(t *testing.T) {
	rv := mustRule(t, GroupRule{Param: GroupParamVolume, Delta: -0.25, EveryN: 1})
	rd := mustRule(t, GroupRule{Param: GroupParamDuration, Delta: 0.5, EveryN: 2})
	idx := idxFor(&NodeGroup{ID: 1, NodeIDs: []NodeID{3}, Rules: []GroupRule{rv, rd}, RulesEnabled: true})
	pd, vm, dm := GroupEffect(idx, 3, 2, 0, 1)
	if pd != 0 {
		t.Errorf("pitch: got %v", pd)
	}
	if vm != 0.5 { // 1 + (-0.25*2)
		t.Errorf("volMul: got %v, want 0.5", vm)
	}
	if dm != 1.5 { // 1 + (0.5*1)
		t.Errorf("durMul: got %v, want 1.5", dm)
	}
	// Volume ramp clamps at its default Min=0, never negative.
	_, vm, _ = GroupEffect(idx, 3, 100, 0, 1)
	if vm != 0 {
		t.Errorf("volMul at round 100: got %v, want 0 (clamped)", vm)
	}
}

func TestGroupEffectStacking(t *testing.T) {
	r1 := mustRule(t, GroupRule{Param: GroupParamPitch, Delta: 1, EveryN: 1})
	r2 := mustRule(t, GroupRule{Param: GroupParamPitch, Delta: 2, EveryN: 2})
	rv1 := mustRule(t, GroupRule{Param: GroupParamVolume, Delta: -0.1, EveryN: 1})
	rv2 := mustRule(t, GroupRule{Param: GroupParamVolume, Delta: -0.1, EveryN: 1})
	idx := idxFor(
		&NodeGroup{ID: 1, NodeIDs: []NodeID{5}, Rules: []GroupRule{r1, rv1}, RulesEnabled: true},
		&NodeGroup{ID: 2, NodeIDs: []NodeID{5}, Rules: []GroupRule{r2, rv2}, RulesEnabled: true},
	)
	pd, vm, _ := GroupEffect(idx, 5, 4, 0, 1)
	if pd != 8 { // 1*4 + 2*2 — pitch deltas SUM across groups
		t.Errorf("stacked pitch: got %v, want 8", pd)
	}
	want := 0.6 * 0.6 // multipliers MULTIPLY across groups
	if diff := vm - want; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("stacked volume: got %v, want %v", vm, want)
	}
}

func TestGroupEffectDurationStacksAcrossGroups(t *testing.T) {
	rd1 := mustRule(t, GroupRule{Param: GroupParamDuration, Delta: 0.5, EveryN: 1})
	rd2 := mustRule(t, GroupRule{Param: GroupParamDuration, Delta: 0.5, EveryN: 1})
	idx := idxFor(
		&NodeGroup{ID: 1, NodeIDs: []NodeID{7}, Rules: []GroupRule{rd1}, RulesEnabled: true},
		&NodeGroup{ID: 2, NodeIDs: []NodeID{7}, Rules: []GroupRule{rd2}, RulesEnabled: true},
	)
	_, _, durMul := GroupEffect(idx, 7, 1, 0, 1)
	want := 1.5 * 1.5 // (1 + 0.5*1) * (1 + 0.5*1)
	if diff := durMul - want; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("duration stacking: got %v, want %v", durMul, want)
	}
}

func TestGroupEffectIdentityAtRoundZeroWithCustomClamp(t *testing.T) {
	// Pitch rule with custom clamp Min:5, Max:24
	rp := mustRule(t, GroupRule{Param: GroupParamPitch, Delta: 2, EveryN: 4, Min: 5, Max: 24})
	// Volume rule with custom clamp Min:2, Max:4
	rv := mustRule(t, GroupRule{Param: GroupParamVolume, Delta: 0.1, EveryN: 1, Min: 2, Max: 4})
	idx := idxFor(&NodeGroup{ID: 1, NodeIDs: []NodeID{8}, Rules: []GroupRule{rp, rv}, RulesEnabled: true})

	// At round 0, identity should hold: (0, 1, 1) regardless of clamp values
	pd, vm, dm := GroupEffect(idx, 8, 0, 0, 1)
	if pd != 0 {
		t.Errorf("pitch at round 0: got %v, want 0 (identity, not clamped to Min:5)", pd)
	}
	if vm != 1 {
		t.Errorf("volume at round 0: got %v, want 1 (identity, not clamped to Min:2)", vm)
	}
	if dm != 1 {
		t.Errorf("duration at round 0: got %v, want 1 (identity)", dm)
	}
}

// loopLen=1 so GroupRound(idx) == idx: an anchor at 10 counts steps from ITS round, not the origin.
func TestGroupEffectAnchoredMidTimeline(t *testing.T) {
	r := mustRule(t, GroupRule{Param: GroupParamPitch, Delta: 1, EveryN: 1, AnchorIdx: 10})
	idx := idxFor(&NodeGroup{ID: 1, NodeIDs: []NodeID{7}, Rules: []GroupRule{r}, RulesEnabled: true})
	for _, c := range []struct {
		absIdx int
		want   float64
	}{
		{10, 0}, // at the anchor itself: 0 steps elapsed
		{11, 1},
		{12, 2},
		{20, 10},
	} {
		pd, _, _ := GroupEffect(idx, 7, c.absIdx, 0, 1)
		if pd != c.want {
			t.Errorf("absIdx %d: got pitch %v, want %v", c.absIdx, pd, c.want)
		}
	}
}

// Before the anchor is identity: min(AnchorIdx, absIdx) collapses the difference to 0.
func TestGroupEffectAnchorBeyondAbsIdxIsIdentity(t *testing.T) {
	r := mustRule(t, GroupRule{Param: GroupParamPitch, Delta: 1, EveryN: 1, AnchorIdx: 10})
	idx := idxFor(&NodeGroup{ID: 1, NodeIDs: []NodeID{7}, Rules: []GroupRule{r}, RulesEnabled: true})
	for _, absIdx := range []int{0, 1, 5, 9, 10} {
		pd, vm, dm := GroupEffect(idx, 7, absIdx, 0, 1)
		if pd != 0 || vm != 1 || dm != 1 {
			t.Errorf("absIdx %d (<= anchor 10): got (%v,%v,%v), want identity (0,1,1)", absIdx, pd, vm, dm)
		}
	}
}

// TestGroupEffectAnchorEveryNInterplay: anchor + EveryN>1 must count rounds
// from the anchor before dividing by EveryN, not the absolute round.
func TestGroupEffectAnchorEveryNInterplay(t *testing.T) {
	r := mustRule(t, GroupRule{Param: GroupParamPitch, Delta: 1, EveryN: 3, AnchorIdx: 10})
	idx := idxFor(&NodeGroup{ID: 1, NodeIDs: []NodeID{7}, Rules: []GroupRule{r}, RulesEnabled: true})
	for _, c := range []struct {
		absIdx int
		want   float64
	}{
		{10, 0},
		{12, 0}, // (12-10)/3 = 0
		{13, 1}, // (13-10)/3 = 1
		{16, 2}, // (16-10)/3 = 2
	} {
		pd, _, _ := GroupEffect(idx, 7, c.absIdx, 0, 1)
		if pd != c.want {
			t.Errorf("absIdx %d: got pitch %v, want %v", c.absIdx, pd, c.want)
		}
	}
}

// TestGroupEffectStackingDifferentAnchors: two pitch rules stacking with
// DIFFERENT anchors — each contributes independently from its own anchor
// round, and the pitch deltas still sum.
func TestGroupEffectStackingDifferentAnchors(t *testing.T) {
	r1 := mustRule(t, GroupRule{Param: GroupParamPitch, Delta: 1, EveryN: 1, AnchorIdx: 0})
	r2 := mustRule(t, GroupRule{Param: GroupParamPitch, Delta: 2, EveryN: 1, AnchorIdx: 8})
	idx := idxFor(
		&NodeGroup{ID: 1, NodeIDs: []NodeID{5}, Rules: []GroupRule{r1}, RulesEnabled: true},
		&NodeGroup{ID: 2, NodeIDs: []NodeID{5}, Rules: []GroupRule{r2}, RulesEnabled: true},
	)
	// At absIdx=10: r1 (anchor 0) has 10 steps -> +10; r2 (anchor 8) has 2
	// steps -> +4. Sum = 14.
	pd, _, _ := GroupEffect(idx, 5, 10, 0, 1)
	if pd != 14 {
		t.Errorf("stacked anchored pitch: got %v, want 14", pd)
	}
	// At absIdx=5: r1 has 5 steps -> +5; r2's anchor (8) is beyond absIdx (5)
	// -> identity contribution (0). Sum = 5.
	pd, _, _ = GroupEffect(idx, 5, 5, 0, 1)
	if pd != 5 {
		t.Errorf("stacked anchored pitch before r2's anchor: got %v, want 5", pd)
	}
}

// AnchorIdx 0 reproduces steps = GroupRound(absIdx) / EveryN exactly.
func TestGroupEffectAnchorZeroIsBackwardCompatible(t *testing.T) {
	r := GroupRule{Param: GroupParamPitch, Delta: 2, EveryN: 4, Min: -24, Max: 24} // AnchorIdx left at zero value
	idx := idxFor(&NodeGroup{ID: 1, NodeIDs: []NodeID{7}, Rules: []GroupRule{r}, RulesEnabled: true})
	for _, c := range []struct {
		round int
		want  float64
	}{{0, 0}, {3, 0}, {4, 2}, {8, 4}, {100, 24}} {
		pd, _, _ := GroupEffect(idx, 7, c.round, 0, 1)
		if pd != c.want {
			t.Errorf("round %d: got %v, want %v", c.round, pd, c.want)
		}
	}
}

func TestBuildGroupIndexDeterministic(t *testing.T) {
	r1 := mustRule(t, GroupRule{Param: GroupParamPitch, Delta: 1, EveryN: 1})
	r2 := mustRule(t, GroupRule{Param: GroupParamPitch, Delta: 2, EveryN: 1})
	g1 := &NodeGroup{ID: 1, NodeIDs: []NodeID{5}, Rules: []GroupRule{r1}, RulesEnabled: true}
	g2 := &NodeGroup{ID: 2, NodeIDs: []NodeID{5}, Rules: []GroupRule{r2}, RulesEnabled: true}
	for i := 0; i < 20; i++ {
		idx := idxFor(g1, g2)
		pd, _, _ := GroupEffect(idx, 5, 1, 0, 1)
		if pd != 3 {
			t.Fatalf("iteration %d: got %v, want 3", i, pd)
		}
	}
	if !idxFor(g1).Member(5) || idxFor(g1).Member(6) {
		t.Fatalf("Member() wrong")
	}
	if !idxFor(g1).HasRules(5) {
		t.Fatalf("HasRules() wrong")
	}
	// Group with members but no rules: Member true, HasRules false.
	g3 := &NodeGroup{ID: 3, NodeIDs: []NodeID{9}}
	idx := idxFor(g3)
	if !idx.Member(9) || idx.HasRules(9) {
		t.Fatalf("no-rule group: Member/HasRules wrong")
	}
}

// Configured Rules on a disabled group contribute nothing, while the group still counts for Member().
func TestBuildGroupIndexSkipsDisabledGroupRules(t *testing.T) {
	r := mustRule(t, GroupRule{Param: GroupParamPitch, Delta: 2, EveryN: 1})
	disabled := &NodeGroup{ID: 1, NodeIDs: []NodeID{7}, Rules: []GroupRule{r}, RulesEnabled: false}
	idx := idxFor(disabled)
	if !idx.Member(7) {
		t.Fatalf("disabled group must still count for Member()")
	}
	if idx.HasRules(7) {
		t.Fatalf("disabled group's rules must not surface via HasRules")
	}
	pd, vm, dm := GroupEffect(idx, 7, 5, 0, 1)
	if pd != 0 || vm != 1 || dm != 1 {
		t.Fatalf("disabled group's rule must be inert, got (%v,%v,%v)", pd, vm, dm)
	}

	// Enabling flips it live without touching Rules.
	enabled := &NodeGroup{ID: 1, NodeIDs: []NodeID{7}, Rules: []GroupRule{r}, RulesEnabled: true}
	idx2 := idxFor(enabled)
	if !idx2.HasRules(7) {
		t.Fatalf("enabled group's rules must surface via HasRules")
	}
	pd2, _, _ := GroupEffect(idx2, 7, 5, 0, 1)
	if pd2 != 10 { // delta 2 * round 5
		t.Fatalf("enabled group pitch = %v, want 10", pd2)
	}
}

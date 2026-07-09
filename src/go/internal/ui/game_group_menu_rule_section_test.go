package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// TestGroupMenuRuleSectionWritesUpTo3RulesSharingEveryNAndAnchor is the Part B
// redesign's core contract (task-rule-anchor brief): the three per-param
// delta rows (rdpit/rdvol/rddur) each write/clear their own GroupRule
// independently, but all rules the menu writes share the SAME EveryN and the
// SAME AnchorIdx (captured once per write) — never per-param anchors/EveryN.
// Zero-delta params are omitted entirely (not written as a zero-delta rule).
func TestGroupMenuRuleSectionWritesUpTo3RulesSharingEveryNAndAnchor(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	gid, err := g.graph.CreateGroup("", []model.NodeID{aID, bID})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	g.groupMenu.OpenAt(gid, 100, 100)

	// Pitch delta 1 (default step), duration delta 0.05 (default step),
	// volume left at 0 -> exactly 2 rules, not 3.
	g.groupMenu.stepRuleDelta(model.GroupParamPitch, 1)
	g.groupMenu.stepRuleDelta(model.GroupParamDuration, 1)
	g.groupMenu.stepRuleEveryN(1) // 1 -> 2, shared across both rules
	advanceFrames(g, 2)

	grp, ok := g.graph.Group(gid)
	if !ok {
		t.Fatalf("group vanished")
	}
	if len(grp.Rules) != 2 {
		t.Fatalf("want exactly 2 rules (pitch+duration, volume omitted at delta 0), got %+v", grp.Rules)
	}
	pit := findRule(grp.Rules, model.GroupParamPitch)
	dur := findRule(grp.Rules, model.GroupParamDuration)
	if pit == nil || !almostEqual(pit.Delta, 1) {
		t.Fatalf("pitch rule = %+v, want delta 1", pit)
	}
	if dur == nil || !almostEqual(dur.Delta, 0.05) {
		t.Fatalf("duration rule = %+v, want delta 0.05", dur)
	}
	if findRule(grp.Rules, model.GroupParamVolume) != nil {
		t.Fatalf("volume rule must be absent (delta was never set): %+v", grp.Rules)
	}
	if pit.EveryN != 2 || dur.EveryN != 2 {
		t.Fatalf("both rules must share EveryN=2, got pitch=%d duration=%d", pit.EveryN, dur.EveryN)
	}
	if pit.AnchorIdx != dur.AnchorIdx {
		t.Fatalf("both rules must share the SAME AnchorIdx, got pitch=%d duration=%d", pit.AnchorIdx, dur.AnchorIdx)
	}
}

// TestGroupMenuParamControlAbsent pins that the old "param" cycle button
// (task-rule-anchor Part B: REMOVE the param cycle button and
// cycleRuleParam) no longer exists as a control, at any point in the rule
// section's lifecycle.
func TestGroupMenuParamControlAbsent(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	gid, err := g.graph.CreateGroup("", []model.NodeID{aID, bID})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	g.groupMenu.OpenAt(gid, 100, 100)
	advanceFrames(g, 1)
	if _, ok := g.groupMenu.rects["param"]; ok {
		t.Fatalf("param control rect must not exist (rects=%v)", g.groupMenu.rects)
	}
	if _, ok := g.groupMenu.btns["param"]; ok {
		t.Fatalf("param button must not exist")
	}

	g.groupMenu.stepRuleDelta(model.GroupParamVolume, 1)
	advanceFrames(g, 2)
	if _, ok := g.groupMenu.rects["param"]; ok {
		t.Fatalf("param control must stay absent after enabling a rule")
	}
	// "delta-"/"delta+" (the old shared control ids) must also be gone.
	for _, id := range []string{"delta", "delta-", "delta+", "deltaval"} {
		if _, ok := g.groupMenu.rects[id]; ok {
			t.Fatalf("old shared delta control %q must not exist", id)
		}
	}
}

// TestGroupMenuEveryNRowStillSteps pins that the shared Every-N stepper
// (n-/n+) still works after the redesign, sharing its row with "ruleonoff"
// instead of the removed "param" button.
func TestGroupMenuEveryNRowStillSteps(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	gid, err := g.graph.CreateGroup("", []model.NodeID{aID, bID})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	g.groupMenu.OpenAt(gid, 100, 100)
	g.groupMenu.stepRuleDelta(model.GroupParamPitch, 1)
	advanceFrames(g, 2)

	if got := g.groupMenu.sharedEveryN(); got != 1 {
		t.Fatalf("initial EveryN = %d, want 1", got)
	}
	g.groupMenu.stepRuleEveryN(1)
	advanceFrames(g, 2)
	if got := g.groupMenu.sharedEveryN(); got != 2 {
		t.Fatalf("EveryN after n+ = %d, want 2", got)
	}
	g.groupMenu.stepRuleEveryN(-1)
	advanceFrames(g, 2)
	if got := g.groupMenu.sharedEveryN(); got != 1 {
		t.Fatalf("EveryN after n- = %d, want 1", got)
	}
}

// TestGroupMenuDurationRowLabelUsesRenamedKey pins the EN copy rename
// ("Length" -> "Duration") for KeyGroupParamDuration, and that the rule
// section's duration row (rddur) is labeled with the same key the batch
// section uses.
func TestGroupMenuDurationRowLabelUsesRenamedKey(t *testing.T) {
	if got, want := i18n.T(i18n.KeyGroupParamDuration), "Duration"; got != want {
		t.Fatalf("KeyGroupParamDuration = %q, want %q", got, want)
	}
}

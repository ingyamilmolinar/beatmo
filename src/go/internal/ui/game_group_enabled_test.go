package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// TestDeltaEditsDoNotEnable is task-enabled-status brief Part C test C1
// (RED): stepping a delta on a fresh provisional group must NEVER flip
// RulesEnabled — the toggle stays Disabled, but the delta is carried in
// Rules regardless.
func TestDeltaEditsDoNotEnable(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	gid := marqueeCreateGroup(t, g, aID, bID)

	g.groupMenu.stepRuleDelta(model.GroupParamPitch, 2)
	advanceFrames(g, 2)

	grp, ok := g.graph.Group(gid)
	if !ok {
		t.Fatalf("group vanished")
	}
	if grp.RulesEnabled {
		t.Fatalf("stepping a delta must NOT enable the group, RulesEnabled=true")
	}
	if got, want := ruleToggleLabel(t, g), i18n.T(i18n.KeyGroupRuleOff); got != want {
		t.Fatalf("toggle label = %q, want Disabled (%q)", got, want)
	}
	if r := findRule(grp.Rules, model.GroupParamPitch); r == nil || !almostEqual(r.Delta, 2) {
		t.Fatalf("pitch rule = %+v, want delta 2 (carried regardless of enabled status)", r)
	}
}

// TestEnableToggleDirtiesSave is task-enabled-status brief Part C test C2
// (RED): a clean, committed, disabled group with deltas already present —
// toggling Enable must dirty the Save button; Save commits exactly 1 undo
// step, re-greys the button, and the model flag flips true.
func TestEnableToggleDirtiesSave(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	gid := createCommittedGroup(t, g, "", []model.NodeID{aID, bID})
	if err := g.graph.SetGroupRules(gid, []model.GroupRule{
		{Param: model.GroupParamPitch, Delta: 3, EveryN: 1},
	}); err != nil {
		t.Fatalf("SetGroupRules: %v", err)
	}

	g.groupMenu.OpenAt(gid, 100, 100)
	drawGroupMenuFrame(g)
	b, ok := g.groupMenu.btns["save"]
	if !ok || b == nil {
		t.Fatalf("save button missing")
	}
	if !b.Disabled {
		t.Fatalf("setup: menu must start clean (save disabled) — deltas alone must not dirty")
	}
	if grp, _ := g.graph.Group(gid); grp.RulesEnabled {
		t.Fatalf("setup: group must start Disabled")
	}

	undoBefore := len(g.undoManager.undo)
	g.groupMenu.setRuleEnabled(true)
	advanceFrames(g, 2)
	drawGroupMenuFrame(g)

	b, ok = g.groupMenu.btns["save"]
	if !ok || b == nil {
		t.Fatalf("save button missing after toggle")
	}
	if b.Disabled {
		t.Fatalf("toggling Enable must dirty the menu (save must be enabled)")
	}

	g.groupMenu.save()
	advanceFrames(g, 2)
	drawGroupMenuFrame(g)

	if got := len(g.undoManager.undo) - undoBefore; got != 1 {
		t.Fatalf("save must record exactly 1 undo step, got delta %d", got)
	}
	b, ok = g.groupMenu.btns["save"]
	if !ok || b == nil {
		t.Fatalf("save button missing after save")
	}
	if !b.Disabled {
		t.Fatalf("save button must be Disabled again immediately after Save commits")
	}
	grp, ok := g.graph.Group(gid)
	if !ok || !grp.RulesEnabled {
		t.Fatalf("model flag must be true after Save, got %+v ok=%v", grp, ok)
	}
}

// TestNewGroupStartsDisabled is task-enabled-status brief Part C test C3
// (RED): a marquee-created group's menu shows Disabled; Save (with no
// toggle) keeps it Disabled — status persists as-is, not implicitly flipped
// by the act of saving.
func TestNewGroupStartsDisabled(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	gid := marqueeCreateGroup(t, g, aID, bID)
	drawGroupMenuFrame(g)

	if got, want := ruleToggleLabel(t, g), i18n.T(i18n.KeyGroupRuleOff); got != want {
		t.Fatalf("fresh provisional group toggle label = %q, want Disabled (%q)", got, want)
	}
	if grp, ok := g.graph.Group(gid); !ok || grp.RulesEnabled {
		t.Fatalf("fresh provisional group must start Disabled, got %+v ok=%v", grp, ok)
	}

	g.groupMenu.stepRuleDelta(model.GroupParamPitch, 1) // configure a delta, never toggle
	advanceFrames(g, 2)
	g.groupMenu.save()
	advanceFrames(g, 2)

	grp, ok := g.graph.Group(gid)
	if !ok {
		t.Fatalf("group must persist after save")
	}
	if grp.RulesEnabled {
		t.Fatalf("Save must not implicitly enable — status persists as-is (Disabled), got RulesEnabled=true")
	}
}

// TestCancelRestoresEnabledFlag is task-enabled-status brief Part C test C6
// (RED): opening a committed (clean, disabled) group, toggling Enable, then
// closing WITHOUT Save must silently revert the flag — no emit, no undo
// step.
func TestCancelRestoresEnabledFlag(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	gid := createCommittedGroup(t, g, "", []model.NodeID{aID, bID})
	if err := g.graph.SetGroupRules(gid, []model.GroupRule{
		{Param: model.GroupParamPitch, Delta: 1, EveryN: 1},
	}); err != nil {
		t.Fatalf("SetGroupRules: %v", err)
	}

	g.groupMenu.OpenAt(gid, 100, 100)
	undoBefore := len(g.undoManager.undo)

	g.groupMenu.setRuleEnabled(true)
	advanceFrames(g, 2)

	grp, ok := g.graph.Group(gid)
	if !ok || !grp.RulesEnabled {
		t.Fatalf("live preview: want RulesEnabled=true, got %+v ok=%v", grp, ok)
	}

	g.groupMenu.Close()

	grp, ok = g.graph.Group(gid)
	if !ok || grp.RulesEnabled {
		t.Fatalf("close-without-save must restore RulesEnabled=false, got %+v ok=%v", grp, ok)
	}
	if got := len(g.undoManager.undo); got != undoBefore {
		t.Fatalf("cancel-restore must not add an undo step, before=%d after=%d", undoBefore, got)
	}
}

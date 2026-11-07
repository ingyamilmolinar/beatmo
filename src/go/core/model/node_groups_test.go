package model

import (
	"errors"
	"testing"
)

func newGroupTestGraph(t *testing.T) (*Graph, NodeID, NodeID, NodeID) {
	t.Helper()
	g := NewGraph(testLogger)
	a := g.AddNode(0, 0, NodeTypeRegular)
	b := g.AddNode(1, 0, NodeTypeRegular)
	c := g.AddNode(2, 0, NodeTypeRegular)
	return g, a, b, c
}

func TestCreateGroupAndRead(t *testing.T) {
	g, a, b, _ := newGroupTestGraph(t)
	id, err := g.CreateGroup("", []NodeID{a, b, a}) // dup a: deduped
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	grp, ok := g.Group(id)
	if !ok {
		t.Fatalf("Group(%d) not found", id)
	}
	if grp.Name == "" {
		t.Fatalf("expected default name, got empty")
	}
	if len(grp.NodeIDs) != 2 {
		t.Fatalf("want 2 members (deduped), got %v", grp.NodeIDs)
	}
	// Read is a copy: mutating it must not affect the graph.
	grp.NodeIDs[0] = 999
	grp2, _ := g.Group(id)
	if grp2.NodeIDs[0] == 999 {
		t.Fatalf("Group() must return a deep copy")
	}
}

func TestCreateGroupValidation(t *testing.T) {
	g, a, _, _ := newGroupTestGraph(t)
	if _, err := g.CreateGroup("x", nil); !errors.Is(err, ErrGroupEmptyNodes) {
		t.Fatalf("want ErrGroupEmptyNodes, got %v", err)
	}
	if _, err := g.CreateGroup("x", []NodeID{a, 12345}); !errors.Is(err, ErrGroupUnknownNode) {
		t.Fatalf("want ErrGroupUnknownNode, got %v", err)
	}
}

func TestGroupsForNodeAndMultiMembership(t *testing.T) {
	g, a, b, c := newGroupTestGraph(t)
	id1, _ := g.CreateGroup("", []NodeID{a, b})
	id2, _ := g.CreateGroup("", []NodeID{a, c})
	got := g.GroupsForNode(a)
	if len(got) != 2 || got[0] != id1 || got[1] != id2 {
		t.Fatalf("GroupsForNode(a) = %v, want [%d %d]", got, id1, id2)
	}
	if got := g.GroupsForNode(c); len(got) != 1 || got[0] != id2 {
		t.Fatalf("GroupsForNode(c) = %v", got)
	}
}

func TestSetGroupRulesValidationAndDefaults(t *testing.T) {
	g, a, _, _ := newGroupTestGraph(t)
	id, _ := g.CreateGroup("", []NodeID{a})
	if err := g.SetGroupRules(id, []GroupRule{{Param: GroupParamNone, Delta: 1, EveryN: 1}}); !errors.Is(err, ErrGroupBadRule) {
		t.Fatalf("bad param: want ErrGroupBadRule, got %v", err)
	}
	if err := g.SetGroupRules(id, []GroupRule{{Param: GroupParamPitch, Delta: 1, EveryN: 0}}); !errors.Is(err, ErrGroupBadRule) {
		t.Fatalf("EveryN<1: want ErrGroupBadRule, got %v", err)
	}
	if err := g.SetGroupRules(id, []GroupRule{{Param: GroupParamPitch, Delta: 1, EveryN: 1, Min: 5, Max: 2}}); !errors.Is(err, ErrGroupBadRule) {
		t.Fatalf("Min>Max: want ErrGroupBadRule, got %v", err)
	}
	// Zero Min/Max → per-param defaults applied on store.
	if err := g.SetGroupRules(id, []GroupRule{{Param: GroupParamPitch, Delta: 2, EveryN: 4}}); err != nil {
		t.Fatalf("SetGroupRules: %v", err)
	}
	grp, _ := g.Group(id)
	if grp.Rules[0].Min != -24 || grp.Rules[0].Max != 24 {
		t.Fatalf("pitch defaults: got Min=%v Max=%v, want -24/24", grp.Rules[0].Min, grp.Rules[0].Max)
	}
	if err := g.SetGroupRules(id, []GroupRule{{Param: GroupParamVolume, Delta: -0.1, EveryN: 2}}); err != nil {
		t.Fatalf("SetGroupRules vol: %v", err)
	}
	grp, _ = g.Group(id)
	if grp.Rules[0].Min != 0 || grp.Rules[0].Max != 4 {
		t.Fatalf("vol defaults: got %v/%v, want 0/4", grp.Rules[0].Min, grp.Rules[0].Max)
	}
}

func TestMembershipMutations(t *testing.T) {
	g, a, b, c := newGroupTestGraph(t)
	id, _ := g.CreateGroup("", []NodeID{a})
	if err := g.AddNodeToGroup(id, b); err != nil {
		t.Fatalf("AddNodeToGroup: %v", err)
	}
	if err := g.AddNodeToGroup(id, b); err != nil { // idempotent
		t.Fatalf("AddNodeToGroup dup: %v", err)
	}
	if err := g.SetGroupNodes(id, []NodeID{b, c}); err != nil {
		t.Fatalf("SetGroupNodes: %v", err)
	}
	if err := g.SetGroupNodes(id, nil); !errors.Is(err, ErrGroupEmptyNodes) {
		t.Fatalf("empty SetGroupNodes: want ErrGroupEmptyNodes, got %v", err)
	}
	if got := g.GroupsForNode(a); len(got) != 0 {
		t.Fatalf("a should have left the group, got %v", got)
	}
	// Removing the last member auto-deletes the group.
	if err := g.RemoveNodeFromGroup(id, b); err != nil {
		t.Fatalf("RemoveNodeFromGroup: %v", err)
	}
	if err := g.RemoveNodeFromGroup(id, c); err != nil {
		t.Fatalf("RemoveNodeFromGroup last: %v", err)
	}
	if _, ok := g.Group(id); ok {
		t.Fatalf("group should be auto-deleted when emptied")
	}
}

func TestRemoveNodeCleansGroups(t *testing.T) {
	g, a, b, _ := newGroupTestGraph(t)
	id, _ := g.CreateGroup("", []NodeID{a, b})
	g.RemoveNode(a)
	grp, ok := g.Group(id)
	if !ok || len(grp.NodeIDs) != 1 || grp.NodeIDs[0] != b {
		t.Fatalf("after RemoveNode(a): %v ok=%v", grp.NodeIDs, ok)
	}
	g.RemoveNode(b)
	if _, ok := g.Group(id); ok {
		t.Fatalf("group must auto-delete when last member node is removed")
	}
}

func TestMoveNodePreservesMembership(t *testing.T) {
	g, a, _, _ := newGroupTestGraph(t)
	id, _ := g.CreateGroup("", []NodeID{a})
	if !g.MoveNode(a, 5, 7) {
		t.Fatalf("MoveNode failed")
	}
	if got := g.GroupsForNode(a); len(got) != 1 || got[0] != id {
		t.Fatalf("membership must survive MoveNode, got %v", got)
	}
}

func TestGroupChangedHookFires(t *testing.T) {
	g, a, b, _ := newGroupTestGraph(t)
	var fired []GroupID
	g.SetGroupChangedHook(func(id GroupID) { fired = append(fired, id) })
	id, _ := g.CreateGroup("", []NodeID{a, b})
	_ = g.SetGroupRules(id, []GroupRule{{Param: GroupParamPitch, Delta: 1, EveryN: 1}})
	_ = g.RenameGroup(id, "melody")
	_ = g.DeleteGroup(id)
	if len(fired) != 4 {
		t.Fatalf("hook should fire once per mutation (create/rules/rename/delete), got %d: %v", len(fired), fired)
	}
	if err := g.DeleteGroup(id); !errors.Is(err, ErrGroupNotFound) {
		t.Fatalf("double delete: want ErrGroupNotFound, got %v", err)
	}
}

// ─── ReanchorAllGroupRules (task-rule-replay brief, test #6) ───────────────

// Every rule stamped, the hook once per TOUCHED group (not per rule, never for rule-less groups), the count returned.
func TestReanchorAllGroupRules(t *testing.T) {
	g, a, b, c := newGroupTestGraph(t)
	id1, err := g.CreateGroup("", []NodeID{a, b})
	if err != nil {
		t.Fatalf("CreateGroup id1: %v", err)
	}
	id2, err := g.CreateGroup("", []NodeID{c}) // stays rule-less — must be skipped
	if err != nil {
		t.Fatalf("CreateGroup id2: %v", err)
	}
	if err := g.SetGroupRules(id1, []GroupRule{
		{Param: GroupParamPitch, Delta: 1, EveryN: 1, AnchorIdx: 100},
		{Param: GroupParamVolume, Delta: 0.1, EveryN: 2, AnchorIdx: 50},
	}); err != nil {
		t.Fatalf("SetGroupRules: %v", err)
	}

	var fired []GroupID
	g.SetGroupChangedHook(func(id GroupID) { fired = append(fired, id) })

	touched := g.ReanchorAllGroupRules(640)

	if touched != 1 {
		t.Fatalf("touched = %d, want 1 (only id1 has rules)", touched)
	}
	if len(fired) != 1 || fired[0] != id1 {
		t.Fatalf("hook fired for %v, want exactly [%d]", fired, id1)
	}
	grp1, ok := g.Group(id1)
	if !ok || len(grp1.Rules) != 2 {
		t.Fatalf("id1 rules = %+v, want 2", grp1.Rules)
	}
	for _, r := range grp1.Rules {
		if r.AnchorIdx != 640 {
			t.Fatalf("rule %+v AnchorIdx not re-anchored to 640", r)
		}
	}
	grp2, ok := g.Group(id2)
	if !ok || len(grp2.Rules) != 0 {
		t.Fatalf("id2 (rule-less) must be untouched, got %+v", grp2.Rules)
	}
}

// TestReanchorAllGroupRulesClampsNegative proves a negative anchor clamps to
// 0 (mirrors normalizeGroupRules' existing AnchorIdx<0 clamp).
func TestReanchorAllGroupRulesClampsNegative(t *testing.T) {
	g, a, _, _ := newGroupTestGraph(t)
	id, err := g.CreateGroup("", []NodeID{a})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	if err := g.SetGroupRules(id, []GroupRule{{Param: GroupParamPitch, Delta: 1, EveryN: 1, AnchorIdx: 5}}); err != nil {
		t.Fatalf("SetGroupRules: %v", err)
	}
	if touched := g.ReanchorAllGroupRules(-42); touched != 1 {
		t.Fatalf("touched = %d, want 1", touched)
	}
	grp, _ := g.Group(id)
	if grp.Rules[0].AnchorIdx != 0 {
		t.Fatalf("negative anchor must clamp to 0, got %d", grp.Rules[0].AnchorIdx)
	}
}

func TestReanchorAllGroupRulesNoGroups(t *testing.T) {
	g := NewGraph(testLogger)
	if touched := g.ReanchorAllGroupRules(10); touched != 0 {
		t.Fatalf("touched = %d, want 0 (no groups)", touched)
	}
}

// Every group rule-less: nothing fires and the count is 0.
func TestReanchorAllGroupRulesSkipsAllRuleLessGroups(t *testing.T) {
	g, a, b, _ := newGroupTestGraph(t)
	if _, err := g.CreateGroup("", []NodeID{a}); err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	if _, err := g.CreateGroup("", []NodeID{b}); err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	var fired []GroupID
	g.SetGroupChangedHook(func(id GroupID) { fired = append(fired, id) })
	if touched := g.ReanchorAllGroupRules(10); touched != 0 {
		t.Fatalf("touched = %d, want 0 (both groups rule-less)", touched)
	}
	if len(fired) != 0 {
		t.Fatalf("hook fired %v, want none", fired)
	}
}

// Directly against g.Groups: empty → 1; {1,2} → 3; {2} → 1; {1,3} → 2.
func TestLowestFreeAllocatorModel(t *testing.T) {
	g := NewGraph(testLogger)
	if got := g.lowestFreeGroupID(); got != 1 {
		t.Fatalf("empty: got %d, want 1", got)
	}
	g.Groups = map[GroupID]*NodeGroup{1: {ID: 1}, 2: {ID: 2}}
	if got := g.lowestFreeGroupID(); got != 3 {
		t.Fatalf("{1,2}: got %d, want 3", got)
	}
	g.Groups = map[GroupID]*NodeGroup{2: {ID: 2}}
	if got := g.lowestFreeGroupID(); got != 1 {
		t.Fatalf("{2}: got %d, want 1", got)
	}
	g.Groups = map[GroupID]*NodeGroup{1: {ID: 1}, 3: {ID: 3}}
	if got := g.lowestFreeGroupID(); got != 2 {
		t.Fatalf("{1,3}: got %d, want 2", got)
	}
}

// Deleting group 2's sole node (auto-delete) frees id 2 for the next create, as an explicit delete of group 1 frees 1.
func TestGroupNumberFillsGapAfterNodeDeletionAutoDelete(t *testing.T) {
	g, a, b, c := newGroupTestGraph(t)

	gid1, err := g.CreateGroup("", []NodeID{a})
	if err != nil || gid1 != 1 {
		t.Fatalf("CreateGroup 1: gid=%d err=%v", gid1, err)
	}
	gid2, err := g.CreateGroup("", []NodeID{b})
	if err != nil || gid2 != 2 {
		t.Fatalf("CreateGroup 2: gid=%d err=%v", gid2, err)
	}
	gid3, err := g.CreateGroup("", []NodeID{c})
	if err != nil || gid3 != 3 {
		t.Fatalf("CreateGroup 3: gid=%d err=%v", gid3, err)
	}

	// Delete group 2's sole member node -> RemoveNode auto-deletes the group.
	g.RemoveNode(b)
	if _, ok := g.Group(gid2); ok {
		t.Fatalf("group 2 must be auto-deleted when its sole member node is removed")
	}

	d := g.AddNode(3, 0, NodeTypeRegular)
	gidNew, err := g.CreateGroup("", []NodeID{d})
	if err != nil {
		t.Fatalf("CreateGroup after auto-delete: %v", err)
	}
	if gidNew != 2 {
		t.Fatalf("new group must reuse the freed number 2, got %d", gidNew)
	}
	grpNew, _ := g.Group(gidNew)
	if grpNew.Name != "Group 2" {
		t.Fatalf("new group name = %q, want %q", grpNew.Name, "Group 2")
	}

	// Delete group 1 directly (mirrors the menu's Delete button) -> next
	// create reuses id 1.
	if err := g.DeleteGroup(gid1); err != nil {
		t.Fatalf("DeleteGroup(1): %v", err)
	}
	e := g.AddNode(4, 0, NodeTypeRegular)
	gidReused, err := g.CreateGroup("", []NodeID{e})
	if err != nil {
		t.Fatalf("CreateGroup after DeleteGroup(1): %v", err)
	}
	if gidReused != 1 {
		t.Fatalf("next create after deleting group 1 must reuse id 1, got %d", gidReused)
	}
}

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// openSidebarGrpExpanded opens the sidebar on node id and expands the grp
// section with a REAL click on its header, mirroring the established "expand
// the section first" pattern used throughout sidebar_group_controls_test.go
// before clicking any of its controls.
//
// Clicking (rather than poking sectionOpen directly) is load-bearing: the
// click is what re-runs NodeSidebar.layout(), the only thing that rebuilds
// sb.rects. layout() runs from Open(), Draw() and HandleInput() — but these
// tests never Draw (advanceFrames only calls Update()), and Open() lays out
// with every section collapsed. A direct poke therefore left sb.rects frozen
// at Open()'s collapsed layout, so the grp rows ("grpnew", "grpchip:<gid>")
// were simply absent. That poke only ever appeared to work because the stub
// build's mock cursor defaults to (0,0) — inside the panel — so the
// dispatcher called HandleInput -> layout() every frame; under the real
// Ebiten build the cursor sits wherever the process left it and the rows
// never laid out. Open() resets every section to collapsed, so this click
// always expands (never collapses) grp.
func openSidebarGrpExpanded(t *testing.T, g *Game, id model.NodeID) {
	t.Helper()
	g.sidebar.Open(g.nodesByID[id])
	clickSidebarControl(t, g, sbSectionHeader(secGrp))
	advanceFrames(g, 1)
	if !g.sidebar.sectionOpen[secGrp] {
		t.Fatalf("openSidebarGrpExpanded: grp section must be expanded after clicking sec-grp")
	}
}

// ─── 1. Section always visible ──────────────────────────────────────────────

// TestSidebarGroupSectionAlwaysVisible is task-sidebar-create brief test 1
// (RED against pre-task code, which gates "sec-grp" on
// len(g.graph.AllGroups())>0): with zero groups anywhere in the project,
// opening the sidebar on a node and expanding the grp section must still lay
// out "sec-grp" and its new "grpnew" row — with no membership chips and no
// "grpadd" row, since there is nothing to add to yet.
func TestSidebarGroupSectionAlwaysVisible(t *testing.T) {
	g, aID, _ := buildGroupSidebarFixture(t)
	if n := len(g.graph.AllGroups()); n != 0 {
		t.Fatalf("setup: want zero groups, got %d", n)
	}
	openSidebarGrpExpanded(t, g, aID)

	if r, ok := g.sidebar.rects[sbSectionHeader(secGrp)]; !ok || r.Empty() {
		t.Fatalf("sec-grp must be present even with zero groups, rects=%v", g.sidebar.rects)
	}
	if r, ok := g.sidebar.rects[sbGroupNew]; !ok || r.Empty() {
		t.Fatalf("grpnew row must be present with zero groups, rects=%v", g.sidebar.rects)
	}
	for id := range g.sidebar.rects {
		if id.el == elGroupChip {
			t.Fatalf("zero groups must produce no chip rows, found %s", id)
		}
	}
	if r, ok := g.sidebar.rects[sbGroupAdd]; ok && !r.Empty() {
		t.Fatalf("zero groups must produce no grpadd row, got rect=%v", r)
	}
}

// ─── 2. grpnew opens a provisional menu ─────────────────────────────────────

// TestSidebarCreateNewGroupOpensProvisionalMenu is task-sidebar-create brief
// test 2 (RED against pre-task code, which has no "grpnew" control at all):
// with zero groups, a real click on grpnew must close the sidebar and open
// the GroupMenu on a brand-new PROVISIONAL group containing EXACTLY the
// selected node, with RulesEnabled false, the lowest-free number
// ("Group 1"), and ZERO undo steps recorded (provisional creation never
// emits — Save owns the single emitGroupCreated step).
func TestSidebarCreateNewGroupOpensProvisionalMenu(t *testing.T) {
	g, aID, _ := buildGroupSidebarFixture(t)
	openSidebarGrpExpanded(t, g, aID)
	undoBefore := len(g.undoManager.undo)

	clickSidebarControl(t, g, sbGroupNew)
	advanceFrames(g, 2)

	if g.sidebar.IsOpen() {
		t.Fatalf("sidebar must close after creating a new group")
	}
	if !g.groupMenu.IsOpen() {
		t.Fatalf("GroupMenu must open after grpnew")
	}
	if !g.groupMenu.provisional {
		t.Fatalf("GroupMenu must be provisional")
	}
	gid := g.groupMenu.GroupID()
	grp, ok := g.graph.Group(gid)
	if !ok {
		t.Fatalf("created group %d must exist", gid)
	}
	if len(grp.NodeIDs) != 1 || grp.NodeIDs[0] != aID {
		t.Fatalf("group members = %v, want exactly [%v]", grp.NodeIDs, aID)
	}
	if grp.RulesEnabled {
		t.Fatalf("RulesEnabled must be false on creation")
	}
	if grp.Name != "Group 1" {
		t.Fatalf("group name = %q, want %q (lowest-free number)", grp.Name, "Group 1")
	}
	if got := len(g.undoManager.undo) - undoBefore; got != 0 {
		t.Fatalf("creating a provisional group must record zero undo steps, got %d", got)
	}
}

// ─── 3. Close without Save leaves no trace ──────────────────────────────────

// TestSidebarCreateThenCloseLeavesNoTrace is task-sidebar-create brief test
// 3: after grpnew opens the provisional menu, closing it WITHOUT Save must
// leave zero trace — zero groups, unchanged undo depth, and the number
// immediately reusable (the next create still mints "Group 1").
func TestSidebarCreateThenCloseLeavesNoTrace(t *testing.T) {
	g, aID, _ := buildGroupSidebarFixture(t)
	openSidebarGrpExpanded(t, g, aID)
	undoBefore := len(g.undoManager.undo)

	clickSidebarControl(t, g, sbGroupNew)
	advanceFrames(g, 2)
	if !g.groupMenu.IsOpen() {
		t.Fatalf("setup: GroupMenu must be open")
	}

	g.groupMenu.Close()

	if n := len(g.graph.AllGroups()); n != 0 {
		t.Fatalf("closing without Save must leave zero groups, got %d", n)
	}
	if got := len(g.undoManager.undo) - undoBefore; got != 0 {
		t.Fatalf("closing without Save must not change undo depth, got %d", got)
	}

	// Reopen: number must still be the lowest-free (1).
	openSidebarGrpExpanded(t, g, aID)
	clickSidebarControl(t, g, sbGroupNew)
	advanceFrames(g, 2)
	grp, ok := g.graph.Group(g.groupMenu.GroupID())
	if !ok || grp.Name != "Group 1" {
		t.Fatalf("next create must still mint Group 1, got %+v (ok=%v)", grp, ok)
	}
}

// ─── 4. Save commits ─────────────────────────────────────────────────────────

// TestSidebarCreateThenSaveCommits is task-sidebar-create brief test 4:
// grpnew -> Save commits the provisional group as exactly ONE undo step; the
// menu stays open (Save greys out); closing after Save persists the group;
// reopening the sidebar shows a membership chip for it.
func TestSidebarCreateThenSaveCommits(t *testing.T) {
	g, aID, _ := buildGroupSidebarFixture(t)
	openSidebarGrpExpanded(t, g, aID)
	undoBefore := len(g.undoManager.undo)

	clickSidebarControl(t, g, sbGroupNew)
	advanceFrames(g, 2)
	gid := g.groupMenu.GroupID()

	clickMenuControl(t, g, gmSave)
	advanceFrames(g, 2)

	if !g.groupMenu.IsOpen() {
		t.Fatalf("menu must stay open after Save")
	}
	if g.groupMenu.provisional {
		t.Fatalf("group must no longer be provisional after Save")
	}
	if got := len(g.undoManager.undo) - undoBefore; got != 1 {
		t.Fatalf("Save must record exactly 1 undo step, got %d", got)
	}

	g.groupMenu.Close()
	if _, ok := g.graph.Group(gid); !ok {
		t.Fatalf("group must persist after closing a saved (non-provisional) menu")
	}

	openSidebarGrpExpanded(t, g, aID)
	if r, ok := g.sidebar.rects[sbGroupChip(gid)]; !ok || r.Empty() {
		t.Fatalf("membership chip for the saved group must be present, rects=%v", g.sidebar.rects)
	}
}

// ─── 5. Coexists with existing membership ───────────────────────────────────

// TestSidebarCreateWithExistingMembership is task-sidebar-create brief test
// 5: a node already belonging to group A can still create a brand-new
// provisional group via grpnew (chips + create coexist); after Save the
// node belongs to both groups (GroupsForNode).
func TestSidebarCreateWithExistingMembership(t *testing.T) {
	g, aID, bID := buildGroupSidebarFixture(t)
	gidA := createCommittedGroup(t, g, "A", []model.NodeID{aID, bID})

	openSidebarGrpExpanded(t, g, aID)
	if r, ok := g.sidebar.rects[sbGroupChip(gidA)]; !ok || r.Empty() {
		t.Fatalf("existing membership chip must coexist with grpnew, rects=%v", g.sidebar.rects)
	}

	clickSidebarControl(t, g, sbGroupNew)
	advanceFrames(g, 2)
	gidNew := g.groupMenu.GroupID()
	if gidNew == gidA {
		t.Fatalf("new group must be distinct from the existing one")
	}
	grpNew, ok := g.graph.Group(gidNew)
	if !ok || len(grpNew.NodeIDs) != 1 || grpNew.NodeIDs[0] != aID {
		t.Fatalf("new provisional group must contain exactly [%v], got %+v (ok=%v)", aID, grpNew, ok)
	}

	clickMenuControl(t, g, gmSave)
	advanceFrames(g, 2)

	memberSet := map[model.GroupID]bool{}
	for _, gid := range g.graph.GroupsForNode(aID) {
		memberSet[gid] = true
	}
	if !memberSet[gidA] || !memberSet[gidNew] {
		t.Fatalf("node must belong to both groups after save, memberships=%v want {%d,%d}", memberSet, gidA, gidNew)
	}
}

// ─── 6. Merge chips appear ───────────────────────────────────────────────────

// TestSidebarCreateShowsMergeChips is task-sidebar-create brief test 6: with
// another group B already existing, grpnew's provisional menu must show a
// merge chip for B (mergeChipCandidates needs no special-casing for a
// single-node provisional selection — see its doc comment); clicking the
// chip merges the node into B and the provisional group is gone.
func TestSidebarCreateShowsMergeChips(t *testing.T) {
	g, aID, bID := buildGroupSidebarFixture(t)
	gidB := createCommittedGroup(t, g, "B", []model.NodeID{bID})

	openSidebarGrpExpanded(t, g, aID)
	clickSidebarControl(t, g, sbGroupNew)
	advanceFrames(g, 2)
	provID := g.groupMenu.GroupID()

	if r, ok := g.groupMenu.rects[gmMergeChip(gidB)]; !ok || r.Empty() {
		t.Fatalf("mergechip for B must be present on the single-node provisional menu, rects=%v", g.groupMenu.rects)
	}

	clickMenuControl(t, g, gmMergeChip(gidB))
	advanceFrames(g, 2)

	grpB, ok := g.graph.Group(gidB)
	if !ok {
		t.Fatalf("group B must still exist")
	}
	memberSet := map[model.NodeID]bool{}
	for _, id := range grpB.NodeIDs {
		memberSet[id] = true
	}
	if !memberSet[aID] || !memberSet[bID] {
		t.Fatalf("B must contain both nodes after merge, got %v", grpB.NodeIDs)
	}
	if _, ok := g.graph.Group(provID); ok {
		t.Fatalf("provisional group must be gone after merging into B")
	}
}

// TestSidebarCreateReusesExactSingleNodeGroup mirrors the marquee's exact-set
// reuse for the sidebar's Create-new-group row (review finding on the
// task-sidebar-create pass): if a COMMITTED group already contains exactly
// this node and nothing else, grpnew must reopen that group non-provisionally
// instead of minting a duplicate — closing the reopened menu must NOT delete
// it, and no new group/undo step may appear.
func TestSidebarCreateReusesExactSingleNodeGroup(t *testing.T) {
	g, aID, _ := buildGroupSidebarFixture(t)
	gid, err := g.graph.CreateGroup("Solo", []model.NodeID{aID})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	undoBefore := len(g.undoManager.undo)

	openSidebarGrpExpanded(t, g, aID)
	clickSidebarControl(t, g, sbGroupNew)
	advanceFrames(g, 2)

	if !g.groupMenu.IsOpen() || g.groupMenu.GroupID() != gid {
		t.Fatalf("grpnew must reopen the existing exact single-node group %d (open=%v gid=%d)", gid, g.groupMenu.IsOpen(), g.groupMenu.GroupID())
	}
	if groups := g.graph.AllGroups(); len(groups) != 1 {
		t.Fatalf("no duplicate group may be created, got %d groups", len(groups))
	}
	// Non-provisional reuse: closing must not delete the committed group.
	g.groupMenu.Close()
	advanceFrames(g, 1)
	if _, ok := g.graph.Group(gid); !ok {
		t.Fatalf("closing the reused menu must not delete the committed group")
	}
	if got := len(g.undoManager.undo); got != undoBefore {
		t.Fatalf("reuse must record no undo step: %d -> %d", undoBefore, got)
	}
}

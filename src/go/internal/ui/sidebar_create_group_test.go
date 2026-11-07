package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// A real header click, never a sectionOpen poke: the click re-runs layout(), the only thing that rebuilds
// sb.rects, and these tests never Draw. A poke left the rows absent, hidden on the stub by its (0,0) cursor.
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

// With zero groups the grp section still lays out its "grpnew" row, with no chips and no "grpadd".
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

// grpnew closes the sidebar and opens the menu on a provisional group of exactly this node, rules off,
// "Group 1", with no undo step: Save owns the single one.
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

// Closing without Save leaves no trace, and the number is reusable at once.
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

// One undo step; the menu stays open with Save greyed; reopening the sidebar shows the chip.
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

// A member of A can still create a new group; after Save it belongs to both.
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

// The provisional menu shows a merge chip for B; clicking it merges the node into B and the provisional group
// is gone.
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

// A committed group of exactly this node is reopened, never duplicated: closing it keeps it, and no group or
// undo step is added.
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

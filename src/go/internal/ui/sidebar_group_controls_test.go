package ui

import (
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// Taller than 800x600: the Groups section renders last, so its rows would sit past the viewport.
func buildGroupSidebarFixture(t *testing.T) (*Game, model.NodeID, model.NodeID) {
	t.Helper()
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 1400)
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

// Emitted like the real commit path: CreateGroup alone records no undo step, so a later Undo would go past the
// group's creation.
func createCommittedGroup(t *testing.T, g *Game, name string, nodeIDs []model.NodeID) model.GroupID {
	t.Helper()
	gid, err := g.graph.CreateGroup(name, nodeIDs)
	if err != nil {
		t.Fatalf("CreateGroup(%q): %v", name, err)
	}
	emitGroupCreated(gid, name, nodeIDs)
	return gid
}

// Through the real Game.Update path, re-laid-out first, so successive clicks need no layout call between them.
func clickSidebarControl(t *testing.T, g *Game, id sbID) {
	t.Helper()
	g.sidebar.layout()
	r, ok := g.sidebar.rects[id]
	if !ok || r.Empty() {
		t.Fatalf("clickSidebarControl(%q): rect missing/empty (rects=%v)", id, g.sidebar.rects)
	}
	cx, cy := (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2
	gridClick(g, cx, cy)
}

// ─── 1. Remove from group (real click, via the grpremove selector) ─────────

// Fails if the dropdown does not open.
func clickRemoveOption(t *testing.T, g *Game, gid model.GroupID) {
	t.Helper()
	clickSidebarControl(t, g, sbGroupRemove)
	advanceFrames(g, 1)
	if !g.sidebar.groupRemoveDropdownOpen {
		t.Fatalf("precondition: groupRemoveDropdownOpen should be true after clicking grpremove")
	}
	clickSidebarControl(t, g, sbGroupRemoveOption(gid))
}

// Exactly the node's memberships, by name.
func TestRemoveSelectorListsOnlyMemberships(t *testing.T) {
	g, aID, bID := buildGroupSidebarFixture(t)
	gid1 := createCommittedGroup(t, g, "G1", []model.NodeID{aID, bID})
	gid2 := createCommittedGroup(t, g, "G2", []model.NodeID{aID})
	gid3 := createCommittedGroup(t, g, "G3", []model.NodeID{bID})
	openSidebarGrpExpanded(t, g, aID)

	clickSidebarControl(t, g, sbGroupRemove)
	advanceFrames(g, 1)
	g.sidebar.layout()

	got := map[model.GroupID]bool{}
	for id := range g.sidebar.rects {
		if gid := model.GroupID(id.arg); id.el == elGroupRemoveOpt {
			got[gid] = true
		}
	}
	want := map[model.GroupID]bool{gid1: true, gid2: true}
	if len(got) != len(want) {
		t.Fatalf("dropdown rows = %v, want exactly %v", got, want)
	}
	for gid := range want {
		if !got[gid] {
			t.Fatalf("dropdown missing row for gid=%d, got %v", gid, got)
		}
	}
	if got[gid3] {
		t.Fatalf("dropdown must not list gid3=%d (node is not a member)", gid3)
	}
	for _, gid := range []model.GroupID{gid1, gid2} {
		id := sbGroupRemoveOption(gid)
		btn, ok := g.sidebar.btns[id]
		if !ok || btn == nil {
			t.Fatalf("missing button for %s", id)
		}
		grp, ok := g.graph.Group(gid)
		if !ok {
			t.Fatalf("group %d vanished", gid)
		}
		if btn.Text != grp.Name {
			t.Fatalf("%s label = %q, want %q", id, btn.Text, grp.Name)
		}
	}
}

// Only group1 loses the node, in one undo step; the dropdown closes and group1's chip goes, while the ring
// stays (still in group2).
func TestRemoveSelectorRemovesMembership(t *testing.T) {
	g, aID, bID := buildGroupSidebarFixture(t)
	gid1 := createCommittedGroup(t, g, "G1", []model.NodeID{aID, bID})
	gid2 := createCommittedGroup(t, g, "G2", []model.NodeID{aID})
	openSidebarGrpExpanded(t, g, aID)

	undoBefore := len(g.undoManager.undo)
	clickRemoveOption(t, g, gid1)
	advanceFrames(g, 2)

	grpIDs := g.graph.GroupsForNode(aID)
	for _, gid := range grpIDs {
		if gid == gid1 {
			t.Fatalf("node must no longer belong to gid1=%d, got %v", gid1, grpIDs)
		}
	}
	found2 := false
	for _, gid := range grpIDs {
		if gid == gid2 {
			found2 = true
		}
	}
	if !found2 {
		t.Fatalf("node must still belong to gid2=%d, got %v", gid2, grpIDs)
	}
	if got := len(g.undoManager.undo) - undoBefore; got != 1 {
		t.Fatalf("expected exactly 1 undo step, got %d", got)
	}
	if lbl := g.undoManager.UndoLabel(); lbl != "edit group" {
		t.Fatalf("undo label = %q, want %q", lbl, "edit group")
	}
	if !g.groupIndexSnapshot().Member(aID) {
		t.Fatalf("groupIndexSnapshot().Member(aID) should still be true (still in gid2)")
	}
	if g.sidebar.groupRemoveDropdownOpen {
		t.Fatalf("dropdown must close after selecting a group")
	}
	g.sidebar.layout()
	if r, ok := g.sidebar.rects[sbGroupChip(gid1)]; ok && !r.Empty() {
		t.Fatalf("chip row for gid1 must be gone next frame, got rect=%v", r)
	}
}

// ─── 2. Remove last member auto-deletes the group ───────────────────────────

// A sole-member group is deleted, in one "delete group" undo step that restores it.
func TestRemoveSelectorAutoDeleteOnEmpty(t *testing.T) {
	g, aID, _ := buildGroupSidebarFixture(t)
	gid := createCommittedGroup(t, g, "Solo", []model.NodeID{aID})
	openSidebarGrpExpanded(t, g, aID)

	undoBefore := len(g.undoManager.undo)
	clickRemoveOption(t, g, gid)
	advanceFrames(g, 2)

	for _, grp := range g.graph.AllGroups() {
		if grp.ID == gid {
			t.Fatalf("group %d must be auto-deleted after removing its last member", gid)
		}
	}
	if got := len(g.undoManager.undo) - undoBefore; got != 1 {
		t.Fatalf("expected exactly 1 undo step, got %d", got)
	}
	if lbl := g.undoManager.UndoLabel(); lbl != "delete group" {
		t.Fatalf("undo label = %q, want %q", lbl, "delete group")
	}

	g.undoManager.Undo()

	if len(g.graph.GroupsForNode(aID)) != 1 {
		t.Fatalf("undo must restore node %d's membership, got groups=%v", aID, g.graph.GroupsForNode(aID))
	}
}

func TestRemoveSelectorHiddenWhenNoMemberships(t *testing.T) {
	g, aID, bID := buildGroupSidebarFixture(t)
	createCommittedGroup(t, g, "G1", []model.NodeID{bID})
	openSidebarGrpExpanded(t, g, aID)
	g.sidebar.layout()
	if r, ok := g.sidebar.rects[sbGroupRemove]; ok && !r.Empty() {
		t.Fatalf("grpremove row must be absent with zero memberships, got rect=%v", r)
	}
}

// The grpremove dropdown owns removal; a chip click still opens the menu.
func TestChipsAreOpenOnlyFullWidth(t *testing.T) {
	g, aID, bID := buildGroupSidebarFixture(t)
	gid := createCommittedGroup(t, g, "G1", []model.NodeID{aID, bID})
	openSidebarGrpExpanded(t, g, aID)
	g.sidebar.layout()

	for id := range g.sidebar.rects {
		if strings.HasPrefix(id.String(), "grpdel:") {
			t.Fatalf("grpdel machinery must be fully removed, found rect id=%s", id)
		}
	}
	for id := range g.sidebar.btns {
		if strings.HasPrefix(id.String(), "grpdel:") {
			t.Fatalf("grpdel machinery must be fully removed, found button id=%s", id)
		}
	}

	clickSidebarControl(t, g, sbGroupChip(gid))
	advanceFrames(g, 1)
	if !g.groupMenu.IsOpen() || g.groupMenu.GroupID() != gid {
		t.Fatalf("chip click must open the GroupMenu for gid=%d, open=%v menuGid=%d", gid, g.groupMenu.IsOpen(), g.groupMenu.GroupID())
	}
}

// ─── 3. Add to group (real click through the dropdown) ─────────────────────

// One undo step, the dropdown closes, and grpadd stays while "Y" remains a candidate.
func TestSidebarAddToGroup_Click(t *testing.T) {
	g, aID, bID := buildGroupSidebarFixture(t)
	gidX := createCommittedGroup(t, g, "X", []model.NodeID{bID})
	gidY := createCommittedGroup(t, g, "Y", []model.NodeID{bID})
	g.sidebar.Open(g.nodesByID[aID])
	g.sidebar.sectionOpen[secGrp] = true
	advanceFrames(g, 1)

	clickSidebarControl(t, g, sbGroupAdd)
	advanceFrames(g, 1)
	if !g.sidebar.groupAddDropdownOpen {
		t.Fatalf("precondition: groupAddDropdownOpen should be true after clicking grpadd")
	}

	undoBefore := len(g.undoManager.undo)
	clickSidebarControl(t, g, sbGroupAddOption(gidX))
	advanceFrames(g, 2)

	found := false
	for _, gid := range g.graph.GroupsForNode(aID) {
		if gid == gidX {
			found = true
		}
	}
	if !found {
		t.Fatalf("node must be a member of group X (gid=%d) after clicking its dropdown row", gidX)
	}
	if got := len(g.undoManager.undo) - undoBefore; got != 1 {
		t.Fatalf("expected exactly 1 undo step, got %d", got)
	}
	if g.sidebar.groupAddDropdownOpen {
		t.Fatalf("dropdown must close after selecting a group")
	}
	g.sidebar.layout()
	if r, ok := g.sidebar.rects[sbGroupAdd]; !ok || r.Empty() {
		t.Fatalf("grpadd row must still be visible: group Y remains a candidate")
	}
	if r, ok := g.sidebar.rects[sbGroupAddOption(gidY)]; ok && !r.Empty() {
		t.Fatalf("dropdown must be closed; grpaddopt rows must not be laid out, got rect for gidY=%v", r)
	}
}

// ─── 4. Add dropdown lists only non-member groups ───────────────────────────

// Exactly the non-member groups, by name.
func TestAddSelectorListsOnlyNonMembers(t *testing.T) {
	g, aID, bID := buildGroupSidebarFixture(t)
	gid1 := createCommittedGroup(t, g, "G1", []model.NodeID{aID})
	gid2 := createCommittedGroup(t, g, "G2", []model.NodeID{bID})
	gid3 := createCommittedGroup(t, g, "G3", []model.NodeID{bID})

	g.sidebar.Open(g.nodesByID[aID])
	g.sidebar.sectionOpen[secGrp] = true
	advanceFrames(g, 1)

	clickSidebarControl(t, g, sbGroupAdd)
	advanceFrames(g, 1)
	g.sidebar.layout()

	got := map[model.GroupID]bool{}
	for id := range g.sidebar.rects {
		if gid := model.GroupID(id.arg); id.el == elGroupAddOpt {
			got[gid] = true
		}
	}
	want := map[model.GroupID]bool{gid2: true, gid3: true}
	if len(got) != len(want) {
		t.Fatalf("dropdown rows = %v, want exactly %v", got, want)
	}
	for gid := range want {
		if !got[gid] {
			t.Fatalf("dropdown missing row for gid=%d, got %v", gid, got)
		}
	}
	if got[gid1] {
		t.Fatalf("dropdown must not list gid1=%d (node is already a member)", gid1)
	}

	for _, gid := range []model.GroupID{gid2, gid3} {
		id := sbGroupAddOption(gid)
		btn, ok := g.sidebar.btns[id]
		if !ok || btn == nil {
			t.Fatalf("missing button for %s", id)
		}
		grp, ok := g.graph.Group(gid)
		if !ok {
			t.Fatalf("group %d vanished", gid)
		}
		if btn.Text != grp.Name {
			t.Fatalf("%s label = %q, want %q", id, btn.Text, grp.Name)
		}
	}
}

// ─── 5. Section visibility ───────────────────────────────────────────────────

func TestSidebarGrpSectionVisibility(t *testing.T) {
	// The section always shows: zero groups still get "grpnew", with no chips and no "grpadd".
	t.Run("no_groups_shows_create_only", func(t *testing.T) {
		g, aID, _ := buildGroupSidebarFixture(t)
		g.sidebar.Open(g.nodesByID[aID])
		g.sidebar.sectionOpen[secGrp] = true
		g.sidebar.layout()
		if r, ok := g.sidebar.rects[sbSectionHeader(secGrp)]; !ok || r.Empty() {
			t.Fatalf("sec-grp must be present even with zero groups")
		}
		if r, ok := g.sidebar.rects[sbGroupNew]; !ok || r.Empty() {
			t.Fatalf("grpnew row must be present with zero groups")
		}
		if r, ok := g.sidebar.rects[sbGroupAdd]; ok && !r.Empty() {
			t.Fatalf("grpadd row must be absent with zero groups, got rect=%v", r)
		}
		for id := range g.sidebar.rects {
			if id.el == elGroupChip {
				t.Fatalf("zero groups must produce no chip rows, found %s", id)
			}
		}
	})

	t.Run("non_member_shows_add_only", func(t *testing.T) {
		g, aID, bID := buildGroupSidebarFixture(t)
		createCommittedGroup(t, g, "G1", []model.NodeID{bID})
		g.sidebar.Open(g.nodesByID[aID])
		g.sidebar.sectionOpen[secGrp] = true
		g.sidebar.layout()
		if r, ok := g.sidebar.rects[sbSectionHeader(secGrp)]; !ok || r.Empty() {
			t.Fatalf("sec-grp must be present when a group exists in the graph")
		}
		if r, ok := g.sidebar.rects[sbGroupAdd]; !ok || r.Empty() {
			t.Fatalf("grpadd row must be present for a non-member node")
		}
		for id := range g.sidebar.rects {
			if id.el == elGroupChip {
				t.Fatalf("non-member node must have no chip rows, found %s", id)
			}
		}
	})

	t.Run("member_shows_chip_and_remove", func(t *testing.T) {
		g, aID, bID := buildGroupSidebarFixture(t)
		gid := createCommittedGroup(t, g, "G1", []model.NodeID{aID, bID})
		g.sidebar.Open(g.nodesByID[aID])
		g.sidebar.sectionOpen[secGrp] = true
		g.sidebar.layout()
		if r, ok := g.sidebar.rects[sbGroupChip(gid)]; !ok || r.Empty() {
			t.Fatalf("chip row must be present for a member node")
		}
		if r, ok := g.sidebar.rects[sbGroupRemove]; !ok || r.Empty() {
			t.Fatalf("grpremove row must be present for a member node")
		}
	})
}

// ─── 6. Add/remove during playback ──────────────────────────────────────────

// Adding to a group with a pitch rule shows in evalNodeParamsOnly at a future round, removing clears it, and
// playback survives both.
func TestSidebarAddRemoveDuringPlayback(t *testing.T) {
	g, aID, bID := buildGroupSidebarFixture(t)
	gid := createCommittedGroup(t, g, "Rule", []model.NodeID{bID})
	if err := g.graph.SetGroupRules(gid, []model.GroupRule{
		{Param: model.GroupParamPitch, Delta: 2, EveryN: 1},
	}); err != nil {
		t.Fatalf("SetGroupRules: %v", err)
	}
	if err := g.graph.SetGroupRulesEnabled(gid, true); err != nil {
		t.Fatalf("SetGroupRulesEnabled: %v", err)
	}

	loopStart := g.loopStartByRow[0]
	loopLen := len(g.beatInfosByRow[0]) - loopStart
	if !g.isLoopByRow[0] || loopLen <= 0 {
		t.Fatalf("expected loop row, isLoop=%v loopLen=%d", g.isLoopByRow[0], loopLen)
	}
	infoAt := func(idx int) model.BeatInfo { return g.beatInfoAtRow(0, idx) }
	aOffset := -1
	for k := 0; k < loopLen; k++ {
		if infoAt(loopStart+k).NodeID == aID {
			aOffset = k
			break
		}
	}
	if aOffset < 0 {
		t.Fatalf("could not find node a's slot within the loop")
	}
	idxRound2 := loopStart + aOffset + 2*loopLen

	g.SetPlaying(true)
	t.Cleanup(func() { stopPlaybackForTest(g) })
	advanceFrames(g, 5)

	_, pBefore, _ := g.evalNodeParamsOnly(0, idxRound2, infoAt(idxRound2))
	if pBefore != 0 {
		t.Fatalf("precondition: node a's pitch must be 0 before joining the rule group, got %v", pBefore)
	}

	g.sidebar.Open(g.nodesByID[aID])
	g.sidebar.sectionOpen[secGrp] = true
	advanceFrames(g, 1)

	clickSidebarControl(t, g, sbGroupAdd)
	advanceFrames(g, 1)
	clickSidebarControl(t, g, sbGroupAddOption(gid))
	advanceFrames(g, 3)

	_, pAfterAdd, _ := g.evalNodeParamsOnly(0, idxRound2, infoAt(idxRound2))
	if pAfterAdd != 4 {
		t.Fatalf("node a's pitch at round 2 after joining the rule group = %v, want 4", pAfterAdd)
	}

	clickRemoveOption(t, g, gid)
	advanceFrames(g, 3)

	_, pAfterRemove, _ := g.evalNodeParamsOnly(0, idxRound2, infoAt(idxRound2))
	if pAfterRemove != 0 {
		t.Fatalf("node a's pitch after leaving the rule group = %v, want 0", pAfterRemove)
	}
	if !g.Playing() {
		t.Fatalf("playback must survive add/remove group edits")
	}
}

// ─── 7. Undo/redo round trip ─────────────────────────────────────────────────

// Counts, not ids: undo/redo reimport the document and may reassign group ids.
func TestRemoveSelectorUndoRedoRoundTrip(t *testing.T) {
	g, aID, bID := buildGroupSidebarFixture(t)
	gid := createCommittedGroup(t, g, "G1", []model.NodeID{aID, bID})
	openSidebarGrpExpanded(t, g, aID)

	if len(g.graph.GroupsForNode(aID)) != 1 {
		t.Fatalf("precondition: node must start a member of exactly 1 group")
	}

	clickRemoveOption(t, g, gid)
	advanceFrames(g, 2)
	if len(g.graph.GroupsForNode(aID)) != 0 {
		t.Fatalf("after remove: node must belong to 0 groups, got %v", g.graph.GroupsForNode(aID))
	}

	g.undoManager.Undo()
	if len(g.graph.GroupsForNode(aID)) != 1 {
		t.Fatalf("after undo: node must be restored to 1 group, got %v", g.graph.GroupsForNode(aID))
	}

	g.undoManager.Redo()
	if len(g.graph.GroupsForNode(aID)) != 0 {
		t.Fatalf("after redo: node must belong to 0 groups again, got %v", g.graph.GroupsForNode(aID))
	}
}

// ─── 9. Four dropdowns are mutually exclusive ────────────────────────────────

// Opening any of the four closes the other three, in every direction.
func TestFourDropdownsMutuallyExclusive(t *testing.T) {
	g, aID, bID := buildGroupSidebarFixture(t)
	createCommittedGroup(t, g, "Member", []model.NodeID{aID})
	createCommittedGroup(t, g, "Candidate", []model.NodeID{bID})
	g.sidebar.Open(g.nodesByID[aID])
	// Only these, or vol/pit/dur/aud overflow the unscrolled viewport at this size.
	g.sidebar.sectionOpen[secLogic] = true
	g.sidebar.sectionOpen[secGroove] = true
	g.sidebar.sectionOpen[secGrp] = true
	advanceFrames(g, 1)

	assertOnlyOpen := func(t *testing.T, want string) {
		t.Helper()
		states := map[string]bool{
			"logic":     g.sidebar.logicDropdownOpen,
			"groove":    g.sidebar.grooveDropdownOpen,
			"grpadd":    g.sidebar.groupAddDropdownOpen,
			"grpremove": g.sidebar.groupRemoveDropdownOpen,
		}
		if !states[want] {
			t.Fatalf("expected %s dropdown open, states=%v", want, states)
		}
		for name, open := range states {
			if name != want && open {
				t.Fatalf("expected only %s open, but %s is also open, states=%v", want, name, states)
			}
		}
	}

	clickSidebarControl(t, g, sbLogicSelect)
	advanceFrames(g, 1)
	assertOnlyOpen(t, "logic")

	clickSidebarControl(t, g, sbGrooveSelect)
	advanceFrames(g, 1)
	assertOnlyOpen(t, "groove")

	clickSidebarControl(t, g, sbGroupAdd)
	advanceFrames(g, 1)
	assertOnlyOpen(t, "grpadd")

	clickSidebarControl(t, g, sbGroupRemove)
	advanceFrames(g, 1)
	assertOnlyOpen(t, "grpremove")

	clickSidebarControl(t, g, sbLogicSelect)
	advanceFrames(g, 1)
	assertOnlyOpen(t, "logic")
}

// ─── 8. Negative cases ───────────────────────────────────────────────────────

// No grpadd once the node is in every group; a remove handler for a group deleted between layout and click
// no-ops with no undo step (invoked directly: real input cannot force the race).
func TestSidebarAddRemoveNegative(t *testing.T) {
	t.Run("grpadd_hidden_when_member_of_all", func(t *testing.T) {
		g, aID, bID := buildGroupSidebarFixture(t)
		createCommittedGroup(t, g, "All", []model.NodeID{aID, bID})
		g.sidebar.Open(g.nodesByID[aID])
		g.sidebar.sectionOpen[secGrp] = true
		g.sidebar.layout()
		if r, ok := g.sidebar.rects[sbGroupAdd]; ok && !r.Empty() {
			t.Fatalf("grpadd row must be absent when node is a member of every group, got rect=%v", r)
		}
	})

	t.Run("remove_vanished_group_no_panic", func(t *testing.T) {
		g, aID, bID := buildGroupSidebarFixture(t)
		gid := createCommittedGroup(t, g, "Solo", []model.NodeID{aID, bID})
		g.sidebar.Open(g.nodesByID[aID])
		g.sidebar.sectionOpen[secGrp] = true
		g.sidebar.layout()

		undoBefore := len(g.undoManager.undo)
		if err := g.graph.DeleteGroup(gid); err != nil {
			t.Fatalf("DeleteGroup: %v", err)
		}
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("removeFromGroup must not panic on a vanished group: %v", r)
				}
			}()
			g.sidebar.removeFromGroup(gid)
		}()
		if got := len(g.undoManager.undo) - undoBefore; got != 0 {
			t.Fatalf("no-op remove on a vanished group must not record an undo step, got %d", got)
		}
	})
}

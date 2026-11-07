package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// The selection pair sits far from every other component so the marquee can never sweep them in.
func buildMergeChipsFixture(t *testing.T, extraComponents int) (g *Game, selA, selB model.NodeID, comps [][2]model.NodeID) {
	t.Helper()
	assertDefaultParityState(t)
	g = New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)
	g.updateBeatInfos()

	mkPair := func(row int) (model.NodeID, model.NodeID) {
		na := g.tryAddNode(0, row, model.NodeTypeRegular)
		nb := g.tryAddNode(1, row, model.NodeTypeRegular)
		g.addEdge(na, nb)
		g.addEdge(nb, na)
		return na.ID, nb.ID
	}

	selA, selB = mkPair(-10)
	comps = make([][2]model.NodeID, extraComponents)
	for i := 0; i < extraComponents; i++ {
		a, b := mkPair(10 * (i + 1))
		comps[i] = [2]model.NodeID{a, b}
	}

	first := g.nodesByID[selA]
	g.start = first
	g.graph.StartNodeID = first.ID
	g.updateBeatInfos()
	return g, selA, selB, comps
}

// Pressed from the BOTTOM-LEFT: a candidate group's badge sits just outside the bbox's top-right corner, where a
// press would arm a badge click instead of the marquee. No group-count assertion: other groups are in play.
func marqueeSelectPair(g *Game, aID, bID model.NodeID) {
	ax1, ay1, _, _ := g.nodeScreenRect(g.nodesByID[aID])
	_, _, bx2, by2 := g.nodeScreenRect(g.nodesByID[bID])
	shiftDrag(g, int(ax1)-20, int(by2)+20, int(bx2)+20, int(ay1)-20)
}

func mergeChipRects(g *Game) map[gmID]bool {
	out := map[gmID]bool{}
	for id := range g.groupMenu.rects {
		if id.el == gmElMergeChip {
			out[id] = true
		}
	}
	return out
}

// Provisional only, one chip per other group lacking some selected member, by GroupID, capped.
func TestMergeChipsShownOnlyWhenProvisionalWithCandidates(t *testing.T) {
	g, selA, selB, comps := buildMergeChipsFixture(t, 6)
	compSuperExtra, compA, compB, compC, compD, compE := comps[0], comps[1], comps[2], comps[3], comps[4], comps[5]

	// A strict superset of the selection, so the marquee still makes a new group; its chip must never appear (a
	// no-op merge).
	gidSuper, err := g.graph.CreateGroup("", []model.NodeID{selA, selB, compSuperExtra[0]})
	if err != nil {
		t.Fatalf("CreateGroup gidSuper: %v", err)
	}
	gid1, err := g.graph.CreateGroup("", []model.NodeID{compA[0], compA[1]})
	if err != nil {
		t.Fatalf("CreateGroup gid1: %v", err)
	}
	gid2, err := g.graph.CreateGroup("", []model.NodeID{compB[0], compB[1]})
	if err != nil {
		t.Fatalf("CreateGroup gid2: %v", err)
	}
	gid3, err := g.graph.CreateGroup("", []model.NodeID{compC[0], compC[1]})
	if err != nil {
		t.Fatalf("CreateGroup gid3: %v", err)
	}
	gid4, err := g.graph.CreateGroup("", []model.NodeID{compD[0], compD[1]})
	if err != nil {
		t.Fatalf("CreateGroup gid4: %v", err)
	}
	gid5, err := g.graph.CreateGroup("", []model.NodeID{compE[0], compE[1]})
	if err != nil {
		t.Fatalf("CreateGroup gid5: %v", err)
	}

	g.groupMenu.OpenAt(gid1, 100, 100)
	advanceFrames(g, 1)
	if chips := mergeChipRects(g); len(chips) != 0 {
		t.Fatalf("committed menu must show no merge chips, found %v", chips)
	}
	g.groupMenu.Close()

	// Marquee-select {selA, selB} -> new provisional group with candidates.
	marqueeSelectPair(g, selA, selB)
	advanceFrames(g, 1)
	if !g.groupMenu.IsOpen() || !g.groupMenu.provisional {
		t.Fatalf("setup: marquee must open a provisional menu, open=%v provisional=%v", g.groupMenu.IsOpen(), g.groupMenu.provisional)
	}

	wantOrder := []model.GroupID{gid1, gid2, gid3, gid4} // ascending, capped at 4
	cands := g.groupMenu.mergeChipCandidates()
	if len(cands) != len(wantOrder) {
		t.Fatalf("want %d candidates (capped at %d), got %d: %+v", len(wantOrder), mergeChipCandidateCap, len(cands), cands)
	}
	for i, want := range wantOrder {
		if cands[i].ID != want {
			t.Fatalf("candidate[%d].ID = %v, want %v (full order=%+v)", i, cands[i].ID, want, cands)
		}
	}

	chips := mergeChipRects(g)
	for _, gid := range wantOrder {
		id := gmMergeChip(gid)
		if !chips[id] {
			t.Fatalf("want chip %q present, chips=%v", id, chips)
		}
	}
	if id := gmMergeChip(gid5); chips[id] {
		t.Fatalf("gid5's chip must be absent (cap at %d by ascending gid), chips=%v", mergeChipCandidateCap, chips)
	}
	if id := gmMergeChip(gidSuper); chips[id] {
		t.Fatalf("superset group's chip must be absent (already contains every selected member), chips=%v", chips)
	}
	if len(chips) != mergeChipCandidateCap {
		t.Fatalf("want exactly %d chips rendered, got %d: %v", mergeChipCandidateCap, len(chips), chips)
	}
}

// One undo step that merges, discards the provisional group (its number released) and reopens on A; undo
// reverts A while the provisional group stays gone.
func TestMergeChipAddsSelectionToExistingGroup(t *testing.T) {
	g, selA, selB, comps := buildMergeChipsFixture(t, 1)
	compA := comps[0]

	gidA, err := g.graph.CreateGroup("", []model.NodeID{compA[0], compA[1]})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	// Baselined as a real Save would: a direct CreateGroup is untracked.
	grpA, _ := g.graph.Group(gidA)
	emitGroupCreated(gidA, grpA.Name, grpA.NodeIDs)
	advanceFrames(g, 2)

	undoBefore := len(g.undoManager.undo)

	marqueeSelectPair(g, selA, selB)
	advanceFrames(g, 1)

	var provID model.GroupID
	for _, grp := range g.graph.AllGroups() {
		if grp.ID != gidA {
			provID = grp.ID
		}
	}
	if provID == 0 {
		t.Fatalf("setup: provisional group not found, groups=%+v", g.graph.AllGroups())
	}
	if !g.groupMenu.IsOpen() || g.groupMenu.GroupID() != provID || !g.groupMenu.provisional {
		t.Fatalf("setup: menu must be open+provisional on the new group %v, got open=%v id=%v provisional=%v",
			provID, g.groupMenu.IsOpen(), g.groupMenu.GroupID(), g.groupMenu.provisional)
	}

	clickMenuControl(t, g, gmMergeChip(gidA))
	advanceFrames(g, 2)

	grpA, ok := g.graph.Group(gidA)
	if !ok {
		t.Fatalf("group A must still exist after merge")
	}
	wantMembers := map[model.NodeID]bool{compA[0]: true, compA[1]: true, selA: true, selB: true}
	if len(grpA.NodeIDs) != len(wantMembers) {
		t.Fatalf("A members after merge = %v, want exactly %v", grpA.NodeIDs, wantMembers)
	}
	for _, id := range grpA.NodeIDs {
		if !wantMembers[id] {
			t.Fatalf("unexpected member %v in A after merge: %v", id, grpA.NodeIDs)
		}
	}

	if _, ok := g.graph.Group(provID); ok {
		t.Fatalf("provisional group %v must be gone after merge", provID)
	}
	if !g.groupMenu.IsOpen() || g.groupMenu.GroupID() != gidA || g.groupMenu.provisional {
		t.Fatalf("menu must reopen on A non-provisional: open=%v id=%v provisional=%v",
			g.groupMenu.IsOpen(), g.groupMenu.GroupID(), g.groupMenu.provisional)
	}
	if got := len(g.undoManager.undo) - undoBefore; got != 1 {
		t.Fatalf("merge must emit exactly 1 undo step, got %d", got)
	}

	// The released number is the lowest free id.
	reuseGid, err := g.graph.CreateGroup("", []model.NodeID{compA[0]})
	if err != nil {
		t.Fatalf("CreateGroup (number-reuse probe): %v", err)
	}
	if reuseGid != provID {
		t.Fatalf("discarded provisional group's number was not released/reused: got %v, want %v", reuseGid, provID)
	}
	_ = g.graph.DeleteGroup(reuseGid) // clean up the probe group

	// Undo the merge: A reverts to {a,b}; the provisional group stays gone.
	g.undoManager.Undo()
	grpA, ok = g.graph.Group(gidA)
	if !ok {
		t.Fatalf("A must still exist after undo")
	}
	wantAfterUndo := map[model.NodeID]bool{compA[0]: true, compA[1]: true}
	if len(grpA.NodeIDs) != len(wantAfterUndo) {
		t.Fatalf("A members after undo = %v, want exactly %v", grpA.NodeIDs, wantAfterUndo)
	}
	for _, id := range grpA.NodeIDs {
		if !wantAfterUndo[id] {
			t.Fatalf("unexpected member %v in A after undo: %v", id, grpA.NodeIDs)
		}
	}
	if _, ok := g.graph.Group(provID); ok {
		t.Fatalf("undo must not resurrect the discarded provisional group %v", provID)
	}
}

// Shown because c is addable though b is already in A; the merge is idempotent.
func TestMergeChipOverlappingSelection(t *testing.T) {
	g, selA, selB, comps := buildMergeChipsFixture(t, 1)
	compA := comps[0]
	aMember := compA[0]

	gidA, err := g.graph.CreateGroup("", []model.NodeID{aMember, selA})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	grpA, _ := g.graph.Group(gidA)
	emitGroupCreated(gidA, grpA.Name, grpA.NodeIDs)
	advanceFrames(g, 2)
	undoBefore := len(g.undoManager.undo)

	marqueeSelectPair(g, selA, selB)
	advanceFrames(g, 1)

	cands := g.groupMenu.mergeChipCandidates()
	found := false
	for _, c := range cands {
		if c.ID == gidA {
			found = true
		}
	}
	if !found {
		t.Fatalf("group A must be an addable candidate (selB is still addable), candidates=%+v", cands)
	}

	clickMenuControl(t, g, gmMergeChip(gidA))
	advanceFrames(g, 2)

	grpA, ok := g.graph.Group(gidA)
	if !ok {
		t.Fatalf("A must still exist")
	}
	want := map[model.NodeID]bool{aMember: true, selA: true, selB: true}
	if len(grpA.NodeIDs) != len(want) {
		t.Fatalf("A members = %v, want exactly %v (idempotent on selA)", grpA.NodeIDs, want)
	}
	for _, id := range grpA.NodeIDs {
		if !want[id] {
			t.Fatalf("unexpected member %v in A: %v", id, grpA.NodeIDs)
		}
	}
	if got := len(g.undoManager.undo) - undoBefore; got != 1 {
		t.Fatalf("overlapping merge must emit exactly 1 undo step, got %d", got)
	}
}

// Provisional rule edits die with the provisional group; the target's rules stay unchanged.
func TestMergeChipRespectsTransactionalRules(t *testing.T) {
	g, selA, selB, comps := buildMergeChipsFixture(t, 1)
	compA := comps[0]

	gidA, err := g.graph.CreateGroup("", []model.NodeID{compA[0], compA[1]})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	if err := g.graph.SetGroupRules(gidA, []model.GroupRule{
		{Param: model.GroupParamVolume, Delta: 0.2, EveryN: 3},
	}); err != nil {
		t.Fatalf("SetGroupRules(A): %v", err)
	}
	if err := g.graph.SetGroupRulesEnabled(gidA, true); err != nil {
		t.Fatalf("SetGroupRulesEnabled(A): %v", err)
	}
	aRulesBefore := append([]model.GroupRule(nil), func() []model.GroupRule { grp, _ := g.graph.Group(gidA); return grp.Rules }()...)

	marqueeSelectPair(g, selA, selB)
	advanceFrames(g, 1)

	clickMenuControl(t, g, gmPlus(gmStepPitch))
	advanceFrames(g, 2)
	clickMenuControl(t, g, gmRuleOnOff)
	advanceFrames(g, 2)

	provID := g.groupMenu.GroupID()
	provGrp, ok := g.graph.Group(provID)
	if !ok || len(provGrp.Rules) == 0 || !provGrp.RulesEnabled {
		t.Fatalf("setup: provisional group must have a pending, enabled rule before the merge, got %+v ok=%v", provGrp, ok)
	}

	clickMenuControl(t, g, gmMergeChip(gidA))
	advanceFrames(g, 2)

	if _, ok := g.graph.Group(provID); ok {
		t.Fatalf("provisional group %v (and its pending rule edits) must be gone after merge", provID)
	}
	grpA, ok := g.graph.Group(gidA)
	if !ok {
		t.Fatalf("A must still exist")
	}
	if len(grpA.Rules) != len(aRulesBefore) {
		t.Fatalf("A's rules must be unchanged by the merge, before=%+v after=%+v", aRulesBefore, grpA.Rules)
	}
	for i := range aRulesBefore {
		if grpA.Rules[i] != aRulesBefore[i] {
			t.Fatalf("A's rules must be unchanged by the merge, before=%+v after=%+v", aRulesBefore, grpA.Rules)
		}
	}
	if !grpA.RulesEnabled {
		t.Fatalf("A's RulesEnabled must be unchanged (still true) by the merge")
	}
}

// The taller chip-bearing layout must still fit the grid pane with every control inside the panel.
func TestGroupMenuFitsWithinGridPaneWithMergeChips(t *testing.T) {
	g, selA, selB, comps := buildMergeChipsFixture(t, 4)

	var gids []model.GroupID
	for _, c := range comps {
		gid, err := g.graph.CreateGroup("", []model.NodeID{c[0], c[1]})
		if err != nil {
			t.Fatalf("CreateGroup: %v", err)
		}
		gids = append(gids, gid)
	}

	marqueeSelectPair(g, selA, selB)
	advanceFrames(g, 1)
	if !g.groupMenu.IsOpen() || !g.groupMenu.provisional {
		t.Fatalf("setup: marquee must open a provisional menu")
	}
	cands := g.groupMenu.mergeChipCandidates()
	if len(cands) != 4 {
		t.Fatalf("setup: want 4 merge-chip candidates, got %d: %+v", len(cands), cands)
	}

	gridRect := g.split.GridRect(g.winW, g.winH)
	panel := g.groupMenu.panel
	if !panel.In(gridRect) {
		t.Fatalf("panel %v not contained in grid pane %v", panel, gridRect)
	}

	controlIDs := []gmID{gmDelete, gmMinus(gmStepEveryN), gmPlus(gmStepEveryN), gmSave, gmClose, gmRuleOnOff, gmMinus(gmStepPitch), gmPlus(gmStepPitch), gmMinus(gmStepVolume), gmPlus(gmStepVolume), gmMinus(gmStepDuration), gmPlus(gmStepDuration)}
	for _, gid := range gids {
		controlIDs = append(controlIDs, gmMergeChip(gid))
	}
	for _, id := range controlIDs {
		r, ok := g.groupMenu.rects[id]
		if !ok || r.Empty() {
			t.Fatalf("control %q rect missing/empty (rects=%v)", id, g.groupMenu.rects)
		}
		if !r.In(panel) {
			t.Fatalf("control %q rect %v not contained in panel %v", id, r, panel)
		}
	}
}

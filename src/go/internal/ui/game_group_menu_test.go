package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// A 3-node loop at a caller-chosen viewport, for layout-fit tests.
func buildGroupLoopCircuitSized(t *testing.T, w, h int) (*Game, model.NodeID, model.NodeID, model.NodeID) {
	t.Helper()
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(w, h)
	g.updateBeatInfos()

	na := g.tryAddNode(0, 0, model.NodeTypeRegular)
	nb := g.tryAddNode(1, 0, model.NodeTypeRegular)
	nc := g.tryAddNode(2, 0, model.NodeTypeRegular)
	g.addEdge(na, nb)
	g.addEdge(nb, nc)
	g.addEdge(nc, na)
	g.start = na
	g.graph.StartNodeID = na.ID
	g.updateBeatInfos()
	return g, na.ID, nb.ID, nc.ID
}

// The panel and every control must render inside the grid pane, as the sidebar does: past it the drum view
// overpaints them and they become unreachable.
func TestGroupMenuFitsWithinGridPane(t *testing.T) {
	sizes := []struct{ w, h int }{{800, 600}, {1280, 720}}
	// Opened provisional, so "save" exists too.
	controlIDs := []gmID{gmDelete, gmMinus(gmStepEveryN), gmPlus(gmStepEveryN), gmSave, gmClose, gmRuleOnOff, gmMinus(gmStepPitch), gmPlus(gmStepPitch), gmMinus(gmStepVolume), gmPlus(gmStepVolume), gmMinus(gmStepDuration), gmPlus(gmStepDuration)}

	// A label overflowing its pill once painted over n- while every rect stayed present, so containment alone
	// cannot catch it.
	stepPairIDs := []gmID{gmMinus(gmStepEveryN), gmValue(gmStepEveryN), gmPlus(gmStepEveryN)}

	for _, sz := range sizes {
		g, aID, bID, cID := buildGroupLoopCircuitSized(t, sz.w, sz.h)
		gid, err := g.graph.CreateGroup("", []model.NodeID{aID, bID, cID})
		if err != nil {
			t.Fatalf("%dx%d: CreateGroup: %v", sz.w, sz.h, err)
		}
		// A live EveryN=4 rule, as in the real post-marquee UI, the value that truncated.
		if err := g.graph.SetGroupRules(gid, []model.GroupRule{{Param: model.GroupParamPitch, Delta: 2, EveryN: 4}}); err != nil {
			t.Fatalf("%dx%d: SetGroupRules: %v", sz.w, sz.h, err)
		}
		gridRect := g.split.GridRect(g.winW, g.winH)

		anchors := [][2]int{
			{gridRect.Dx() - 10, gridRect.Dy() - 10}, // near grid-pane bottom-right
			{20, 20},                                 // near grid-pane top-left
		}
		for _, a := range anchors {
			g.groupMenu.OpenAtProvisional(gid, a[0], a[1])
			advanceFrames(g, 1)

			panel := g.groupMenu.panel
			if !panel.In(gridRect) {
				t.Fatalf("%dx%d anchor %v: panel %v not contained in grid pane %v", sz.w, sz.h, a, panel, gridRect)
			}
			for _, id := range controlIDs {
				r, ok := g.groupMenu.rects[id]
				if !ok || r.Empty() {
					t.Fatalf("%dx%d anchor %v: control %q rect missing/empty (rects=%v)", sz.w, sz.h, a, id, g.groupMenu.rects)
				}
				if !r.In(panel) {
					t.Fatalf("%dx%d anchor %v: control %q rect %v not contained in panel %v", sz.w, sz.h, a, id, r, panel)
				}
			}

			// Every rect exists, non-empty, and the ± buttons at least sidebarIncBtnW wide.
			rects := make(map[gmID]image.Rectangle, len(stepPairIDs))
			for _, id := range stepPairIDs {
				r, ok := g.groupMenu.rects[id]
				if !ok || r.Empty() {
					t.Fatalf("%dx%d anchor %v: %q rect missing/empty", sz.w, sz.h, a, id)
				}
				rects[id] = r
			}
			for _, id := range []gmID{gmMinus(gmStepEveryN), gmPlus(gmStepEveryN)} {
				if w := rects[id].Dx(); w < sidebarIncBtnW {
					t.Fatalf("%dx%d anchor %v: %q width %d < sidebarIncBtnW %d", sz.w, sz.h, a, id, w, sidebarIncBtnW)
				}
			}
			// No pairwise overlap between the six controls sharing the row.
			for i := 0; i < len(stepPairIDs); i++ {
				for j := i + 1; j < len(stepPairIDs); j++ {
					idI, idJ := stepPairIDs[i], stepPairIDs[j]
					if rects[idI].Overlaps(rects[idJ]) {
						t.Fatalf("%dx%d anchor %v: %q %v overlaps %q %v", sz.w, sz.h, a, idI, rects[idI], idJ, rects[idJ])
					}
				}
			}

			// The rendered label must fit its own pill; a full sentence overflowed onto n- at every density.
			label := g.groupMenu.everyNPillLabel(g.groupMenu.sharedEveryN())
			if tw, pw := StyledTextWidth(label, RoleBody), rects[gmValue(gmStepEveryN)].Dx(); tw > pw {
				t.Fatalf("%dx%d anchor %v: everyN pill label %q width %d overflows pill width %d", sz.w, sz.h, a, label, tw, pw)
			}
		}
	}
}

// Delta and EveryN edits write Rules but never flip RulesEnabled; setRuleEnabled touches only the flag.
func TestGroupMenuRuleEditing(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	gid, _ := g.graph.CreateGroup("", []model.NodeID{aID, bID})
	g.groupMenu.OpenAt(gid, 100, 100)

	g.groupMenu.stepRuleDelta(model.GroupParamPitch, +1)
	g.groupMenu.stepRuleEveryN(+1) // 1 -> 2
	advanceFrames(g, 2)

	grp, _ := g.graph.Group(gid)
	if len(grp.Rules) != 1 {
		t.Fatalf("want 1 rule, got %+v", grp.Rules)
	}
	if grp.Rules[0].Param != model.GroupParamPitch {
		t.Fatalf("Param = %v, want pitch", grp.Rules[0].Param)
	}
	if grp.Rules[0].EveryN != 2 {
		t.Fatalf("EveryN = %d, want 2", grp.Rules[0].EveryN)
	}
	if grp.RulesEnabled {
		t.Fatalf("delta/everyN edits must NOT enable the group, got RulesEnabled=true")
	}

	g.groupMenu.setRuleEnabled(true)
	advanceFrames(g, 2)
	grp, _ = g.graph.Group(gid)
	if !grp.RulesEnabled {
		t.Fatalf("setRuleEnabled(true) must flip the flag")
	}
	if len(grp.Rules) != 1 {
		t.Fatalf("enabling must not touch the configured rules, got %+v", grp.Rules)
	}

	g.groupMenu.setRuleEnabled(false)
	advanceFrames(g, 2)
	grp, _ = g.graph.Group(gid)
	if grp.RulesEnabled {
		t.Fatalf("setRuleEnabled(false) must clear the flag")
	}
	if len(grp.Rules) != 1 {
		t.Fatalf("disabling must NOT clear the configured rules, got %+v", grp.Rules)
	}
}

func TestGroupMenuDeleteGroupClosesMenu(t *testing.T) {
	g, aID, _ := buildGroupLoopCircuit(t)
	gid, _ := g.graph.CreateGroup("", []model.NodeID{aID})
	g.groupMenu.OpenAt(gid, 100, 100)
	g.groupMenu.deleteGroup()
	advanceFrames(g, 2)
	if g.groupMenu.IsOpen() {
		t.Fatalf("menu must close when its group is deleted")
	}
	if _, ok := g.graph.Group(gid); ok {
		t.Fatalf("group must be deleted")
	}
}

// The live group name, falling back to the i18n title when unnamed or vanished.
func TestGroupMenuTitleShowsGroupName(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	// CreateGroup auto-names, so the empty-name case goes through RenameGroup, which does not validate.
	gid, _ := g.graph.CreateGroup("temp", []model.NodeID{aID, bID})
	if err := g.graph.RenameGroup(gid, ""); err != nil {
		t.Fatalf("RenameGroup: %v", err)
	}
	g.groupMenu.OpenAt(gid, 100, 100)

	if got, want := g.groupMenu.titleText(), i18n.T(i18n.KeyGroupMenuTitle); got != want {
		t.Fatalf("unnamed group titleText() = %q, want fallback %q", got, want)
	}

	if err := g.graph.RenameGroup(gid, "Bassline"); err != nil {
		t.Fatalf("RenameGroup: %v", err)
	}
	if got, want := g.groupMenu.titleText(), "Bassline"; got != want {
		t.Fatalf("named group titleText() = %q, want %q", got, want)
	}

	g.groupMenu.deleteGroup()
	advanceFrames(g, 2)
	if got, want := g.groupMenu.titleText(), i18n.T(i18n.KeyGroupMenuTitle); got != want {
		t.Fatalf("vanished group titleText() = %q, want fallback %q", got, want)
	}
}

func TestGroupMenuClosesWhenGroupEmptiedByNodeDeletion(t *testing.T) {
	g, aID, _ := buildGroupLoopCircuit(t)
	gid, _ := g.graph.CreateGroup("", []model.NodeID{aID})
	g.groupMenu.OpenAt(gid, 100, 100)
	g.deleteNode(g.nodesByID[aID])
	advanceFrames(g, 2)
	if g.groupMenu.IsOpen() {
		t.Fatalf("menu must close when its group empties/auto-deletes")
	}
}

// A committed group survives Close(); only provisional groups are discarded on close.
func TestChipReopenCloseDoesNotDelete(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	gid, _ := g.graph.CreateGroup("", []model.NodeID{aID, bID})
	g.groupMenu.OpenAt(gid, 100, 100)

	g.groupMenu.Close()

	if g.groupMenu.IsOpen() {
		t.Fatalf("menu must close")
	}
	if _, ok := g.graph.Group(gid); !ok {
		t.Fatalf("committed (non-provisional) group must survive a plain Close()")
	}
}

// Deleting a never-committed group is a silent discard, never a delete event.
func TestProvisionalDeleteButtonSilent(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	undoBefore := len(g.undoManager.undo)
	gid := marqueeCreateGroup(t, g, aID, bID)

	g.groupMenu.deleteGroup()
	advanceFrames(g, 2)

	if g.groupMenu.IsOpen() {
		t.Fatalf("menu must close")
	}
	if _, ok := g.graph.Group(gid); ok {
		t.Fatalf("group must be deleted")
	}
	if got := len(g.undoManager.undo); got != undoBefore {
		t.Fatalf("provisional delete-button must not add an undo step, before=%d after=%d", undoBefore, got)
	}
}

// Deleting the sole member re-enters Close() while still provisional; its "still exists" guard must make that a
// no-op, and the only undo step is the node delete.
func TestProvisionalGroupEmptiedByNodeDeleteNoDoubleDelete(t *testing.T) {
	g, aID, _ := buildGroupLoopCircuit(t)
	// The two nodes are fractions of a pixel apart at default zoom, so the drag ends exactly at node a's far edge.
	ax1, ay1, ax2, ay2 := g.nodeScreenRect(g.nodesByID[aID])
	// Walked until production calls the point empty (no node, badge or guide-snap radius, which would arm a link
	// drag), so the test survives tuning.
	px, py := int(ax1)-10, int(ay1)-10
	for off := 10; ; off += 10 {
		px, py = int(ax1)-off, int(ay1)-off
		if px < 1 || py < 1 {
			t.Fatalf("setup: no empty press point up-left of node a within the pane")
		}
		if g.nodeAtScreen(px, py) == nil && g.guideTargetAt(px, py).Kind != GuideNode {
			if _, onBadge := g.groupBadgeAt(px, py); !onBadge {
				break
			}
		}
	}
	shiftDrag(g, px, py, int(ax2), int(ay2))

	groups := g.graph.AllGroups()
	if len(groups) != 1 || len(groups[0].NodeIDs) != 1 {
		t.Fatalf("setup: want 1 group with 1 member, got %+v", groups)
	}
	gid := groups[0].ID
	if !g.groupMenu.IsOpen() {
		t.Fatalf("setup: menu must be open (provisional)")
	}
	undoBefore := len(g.undoManager.undo)

	g.deleteNode(g.nodesByID[aID])
	advanceFrames(g, 2)

	if g.groupMenu.IsOpen() {
		t.Fatalf("menu must close")
	}
	if _, ok := g.graph.Group(gid); ok {
		t.Fatalf("group must be gone (auto-deleted by node removal)")
	}
	if got := len(g.undoManager.undo) - undoBefore; got != 1 {
		t.Fatalf("undo depth must reflect ONLY the node deletion, got delta %d", got)
	}
}

// No panic, and no phantom group leaks into the imported document.
func TestImportClosesProvisionalMenuSilently(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	// Captured before the marquee creates the provisional group.
	data, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	marqueeCreateGroup(t, g, aID, bID)

	if err := g.Import(data); err != nil {
		t.Fatalf("Import: %v", err)
	}

	if g.groupMenu.IsOpen() {
		t.Fatalf("Import must close the group menu")
	}
	if n := len(g.graph.AllGroups()); n != 0 {
		t.Fatalf("imported doc's group set must be exactly what the file says (none), got %d", n)
	}
}

// Save always exists: disabled right after OpenAt, enabled by an edit, disabled again by Save, menu still open.
func TestSaveButtonAlwaysPresent(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	gid, err := g.graph.CreateGroup("", []model.NodeID{aID, bID})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}

	g.groupMenu.OpenAt(gid, 100, 100) // committed (non-provisional)
	drawGroupMenuFrame(g)

	if _, ok := g.groupMenu.rects[gmSave]; !ok {
		t.Fatalf("save control rect must exist for a committed group")
	}
	b, ok := g.groupMenu.btns[gmSave]
	if !ok || b == nil {
		t.Fatalf("save button must be wired for a committed group")
	}
	if !b.Disabled {
		t.Fatalf("save button must start Disabled: no pending edits right after OpenAt")
	}

	g.groupMenu.stepRuleDelta(model.GroupParamPitch, 1)
	advanceFrames(g, 2)
	drawGroupMenuFrame(g)
	b, ok = g.groupMenu.btns[gmSave]
	if !ok || b == nil {
		t.Fatalf("save button missing after edit")
	}
	if b.Disabled {
		t.Fatalf("save button must be enabled after a rule edit (dirty)")
	}

	g.groupMenu.save()
	advanceFrames(g, 2)
	drawGroupMenuFrame(g)

	if !g.groupMenu.IsOpen() {
		t.Fatalf("Save must keep the menu open (task-save-always)")
	}
	b, ok = g.groupMenu.btns[gmSave]
	if !ok || b == nil {
		t.Fatalf("save button missing after Save")
	}
	if !b.Disabled {
		t.Fatalf("save button must be Disabled again immediately after Save commits")
	}
}

// Edits preview live, but Close without Save restores the pre-open snapshot: no undo step, no emit.
func TestCommittedEditsCancelOnClose(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	gid := createCommittedGroup(t, g, "", []model.NodeID{aID, bID})
	if err := g.graph.SetGroupRules(gid, []model.GroupRule{{Param: model.GroupParamPitch, Delta: 1, EveryN: 1}}); err != nil {
		t.Fatalf("SetGroupRules: %v", err)
	}

	g.groupMenu.OpenAt(gid, 100, 100)
	undoBefore := len(g.undoManager.undo)

	g.groupMenu.stepRuleDelta(model.GroupParamPitch, 1) // 1 -> 2
	g.groupMenu.stepRuleDelta(model.GroupParamPitch, 1) // 2 -> 3
	advanceFrames(g, 2)

	grp, ok := g.graph.Group(gid)
	if !ok || len(grp.Rules) != 1 || !almostEqual(grp.Rules[0].Delta, 3) {
		t.Fatalf("live preview: want delta 3, got %+v ok=%v", grp.Rules, ok)
	}

	g.groupMenu.Close()

	grp, ok = g.graph.Group(gid)
	if !ok || len(grp.Rules) != 1 || !almostEqual(grp.Rules[0].Delta, 1) {
		t.Fatalf("close-without-save must restore delta to 1, got %+v ok=%v", grp.Rules, ok)
	}
	if got := len(g.undoManager.undo); got != undoBefore {
		t.Fatalf("cancel-restore must not add an undo step, before=%d after=%d", undoBefore, got)
	}
}

// Three edits then Save are ONE undo step; undo restores pre-open, redo the saved state.
func TestCommittedSaveEmitsOnce(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	gid := createCommittedGroup(t, g, "", []model.NodeID{aID, bID})
	g.groupMenu.OpenAt(gid, 100, 100)
	undoBefore := len(g.undoManager.undo)

	// Separate frames: Update wraps each frame in an undo group, so same-frame edits would coalesce anyway.
	g.groupMenu.stepRuleDelta(model.GroupParamPitch, 1)
	advanceFrames(g, 1)
	g.groupMenu.stepRuleEveryN(1)
	advanceFrames(g, 1)
	g.groupMenu.stepRuleDelta(model.GroupParamVolume, 1)
	advanceFrames(g, 1)

	g.groupMenu.save()
	advanceFrames(g, 2)

	if got := len(g.undoManager.undo) - undoBefore; got != 1 {
		t.Fatalf("save must record exactly 1 undo step for 3 edits, got delta %d", got)
	}
	savedGrp, ok := g.graph.Group(gid)
	if !ok || len(savedGrp.Rules) == 0 {
		t.Fatalf("setup: expected rules to have been saved, got %+v ok=%v", savedGrp.Rules, ok)
	}

	g.undoManager.Undo()
	preOpenGrp, ok := g.graph.Group(gid)
	if !ok || len(preOpenGrp.Rules) != 0 {
		t.Fatalf("undo must restore pre-open state (0 rules), got %+v ok=%v", preOpenGrp.Rules, ok)
	}

	g.undoManager.Redo()
	redoneGrp, ok := g.graph.Group(gid)
	if !ok || !rulesEqual(redoneGrp.Rules, savedGrp.Rules) {
		t.Fatalf("redo must restore the saved state, got %+v want %+v", redoneGrp.Rules, savedGrp.Rules)
	}
}

// The menu stays OPEN after Save; the group persists and exactly one undo step lands.
func TestSaveKeepsMenuOpen(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	undoBefore := len(g.undoManager.undo)
	gid := marqueeCreateGroup(t, g, aID, bID)

	g.groupMenu.stepRuleDelta(model.GroupParamPitch, 1)
	advanceFrames(g, 2)

	g.groupMenu.save()
	advanceFrames(g, 2)
	drawGroupMenuFrame(g)

	if !g.groupMenu.IsOpen() {
		t.Fatalf("Save must keep the menu open")
	}
	if g.groupMenu.GroupID() != gid {
		t.Fatalf("menu must still reference the same group after Save")
	}
	if _, ok := g.graph.Group(gid); !ok {
		t.Fatalf("group must persist after Save")
	}
	if g.groupMenu.provisional {
		t.Fatalf("provisional flag must be cleared after Save")
	}
	b, ok := g.groupMenu.btns[gmSave]
	if !ok || b == nil {
		t.Fatalf("save button missing")
	}
	if !b.Disabled {
		t.Fatalf("save button must be Disabled immediately after Save (no pending edits)")
	}
	if got := len(g.undoManager.undo) - undoBefore; got != 1 {
		t.Fatalf("want exactly 1 undo step (creation), got delta %d", got)
	}
}

// AnchorIdx is runtime ramp state, ignored by the dirty comparison, so a play-start re-anchor never dirties.
func TestAnchorChangesDoNotDirty(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	gid := createCommittedGroup(t, g, "", []model.NodeID{aID, bID})
	if err := g.graph.SetGroupRules(gid, []model.GroupRule{{Param: model.GroupParamPitch, Delta: 1, EveryN: 1, AnchorIdx: 50}}); err != nil {
		t.Fatalf("SetGroupRules: %v", err)
	}

	g.groupMenu.OpenAt(gid, 100, 100)
	drawGroupMenuFrame(g)
	b, ok := g.groupMenu.btns[gmSave]
	if !ok || b == nil {
		t.Fatalf("save button missing")
	}
	if !b.Disabled {
		t.Fatalf("setup: menu must start clean (save disabled)")
	}

	// The real play-start seam (TriggerPlayPause through Game.Update).
	g.playFn = func(id instrument.ID, vol float64, when ...float64) {}
	g.drum.TriggerPlayPause()
	_ = g.Update()
	t.Cleanup(func() { stopPlaybackForTest(g) })
	if !g.Playing() {
		t.Fatalf("setup: not playing after TriggerPlayPause")
	}

	grp, ok := g.graph.Group(gid)
	if !ok || len(grp.Rules) != 1 {
		t.Fatalf("setup: rule must survive play-start, got %+v ok=%v", grp.Rules, ok)
	}
	if grp.Rules[0].AnchorIdx == 50 {
		t.Fatalf("setup: play-start must have re-anchored away from the stale snapshot value 50 for this assertion to be meaningful")
	}

	drawGroupMenuFrame(g)
	b, ok = g.groupMenu.btns[gmSave]
	if !ok || b == nil {
		t.Fatalf("save button missing after play-start")
	}
	if !b.Disabled {
		t.Fatalf("play-start re-anchor must not dirty the menu (AnchorIdx must be ignored by the dirty comparison)")
	}
}

// A stale handle must never resolve to a later group that reused the id: the menu closes on vanish and stays closed.
func TestStaleMenuHandleCannotAliasReusedID(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	gid, err := g.graph.CreateGroup("", []model.NodeID{aID})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	g.groupMenu.OpenAt(gid, 100, 100)
	if !g.groupMenu.IsOpen() || g.groupMenu.GroupID() != gid {
		t.Fatalf("setup: menu must be open on gid %d", gid)
	}

	// An out-of-band delete, not through the menu.
	if err := g.graph.DeleteGroup(gid); err != nil {
		t.Fatalf("DeleteGroup: %v", err)
	}
	advanceFrames(g, 1)
	if g.groupMenu.IsOpen() {
		t.Fatalf("menu must close when its group vanishes externally")
	}

	// A new group reuses the freed lowest id.
	gid2, err := g.graph.CreateGroup("", []model.NodeID{bID})
	if err != nil {
		t.Fatalf("CreateGroup (reuse): %v", err)
	}
	if gid2 != gid {
		t.Fatalf("setup: expected the new group to reuse id %d, got %d", gid, gid2)
	}
	if g.groupMenu.IsOpen() {
		t.Fatalf("menu must stay closed — a stale handle must not alias the reused id %d", gid2)
	}
}

// ─── Enabled/Disabled relabel + live-state sync (task-rule-replay brief, Part 2) ───

// A real Update+Draw, so the label refreshes from live state as the render loop does (Draw → layoutAt).
func drawGroupMenuFrame(g *Game) {
	_ = g.Update()
	img := newTrackedImage("test.groupmenu.sync", g.winW, g.winH)
	defer releaseImage(img)
	g.Draw(img)
}

// ruleToggleLabel reads the ruleonoff button's current displayed text.
func ruleToggleLabel(t *testing.T, g *Game) string {
	t.Helper()
	b, ok := g.groupMenu.btns[gmRuleOnOff]
	if !ok || b == nil {
		t.Fatalf("ruleonoff button missing")
	}
	return b.Text
}

// Through the i18n keys, never literals, so a copy change cannot desync the test.
func TestGroupMenuRuleToggleLabelUsesI18nKeys(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	gid, err := g.graph.CreateGroup("", []model.NodeID{aID, bID})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	g.groupMenu.OpenAt(gid, 100, 100)
	advanceFrames(g, 1)

	if got, want := ruleToggleLabel(t, g), i18n.T(i18n.KeyGroupRuleOff); got != want {
		t.Fatalf("label (no rules) = %q, want %q (KeyGroupRuleOff)", got, want)
	}

	clickMenuControl(t, g, gmPlus(gmStepPitch))
	advanceFrames(g, 2)
	if grp, _ := g.graph.Group(gid); len(grp.Rules) != 1 {
		t.Fatalf("setup: want 1 rule after rdpit+, got %+v", grp.Rules)
	}
	// A delta alone keeps it Disabled.
	if got, want := ruleToggleLabel(t, g), i18n.T(i18n.KeyGroupRuleOff); got != want {
		t.Fatalf("label (delta configured, not enabled) = %q, want %q (KeyGroupRuleOff)", got, want)
	}

	clickMenuControl(t, g, gmRuleOnOff)
	advanceFrames(g, 2)
	if got, want := ruleToggleLabel(t, g), i18n.T(i18n.KeyGroupRuleOn); got != want {
		t.Fatalf("label (rule enabled) = %q, want %q (KeyGroupRuleOn)", got, want)
	}
}

// The label derives from the live group, never stale menu scratch, across a close and reopen.
func TestGroupMenuRuleToggleLabelSyncsOnReopen(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	gid, err := g.graph.CreateGroup("", []model.NodeID{aID, bID})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	g.groupMenu.OpenAt(gid, 100, 100)
	g.groupMenu.stepRuleDelta(model.GroupParamPitch, 1)
	// The toggle is the only path that enables.
	g.groupMenu.setRuleEnabled(true)
	advanceFrames(g, 2)
	if grp, _ := g.graph.Group(gid); len(grp.Rules) != 1 || !grp.RulesEnabled {
		t.Fatalf("setup: want 1 enabled rule, got %+v", grp)
	}
	// Edits are only a preview until Save, so save for the edit to persist across the reopen.
	g.groupMenu.save()
	advanceFrames(g, 2)

	g.groupMenu.Close()
	advanceFrames(g, 1)
	if g.groupMenu.IsOpen() {
		t.Fatalf("setup: menu must be closed")
	}

	g.groupMenu.OpenAt(gid, 100, 100) // reopen, e.g. via the sidebar chip
	advanceFrames(g, 1)
	if got, want := ruleToggleLabel(t, g), i18n.T(i18n.KeyGroupRuleOn); got != want {
		t.Fatalf("label after reopen = %q, want %q (KeyGroupRuleOn) — must read the live group, not stale scratch", got, want)
	}
}

// After an undo the label must read Disabled. Import (the undo restore) always closes the menu, so the
// reopen is the faithful check.
func TestGroupMenuRuleToggleLabelSyncsOnUndo(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	// createCommittedGroup emits the creation, so the single Undo clears only the rule, not the group.
	gid := createCommittedGroup(t, g, "", []model.NodeID{aID, bID})
	g.groupMenu.OpenAt(gid, 100, 100)
	g.groupMenu.stepRuleDelta(model.GroupParamPitch, 1)
	// task-enabled-status brief Part C: the delta alone never enables.
	g.groupMenu.setRuleEnabled(true)
	advanceFrames(g, 2)
	// Without Save there is no rule undo step, and Undo would remove the group's creation instead.
	g.groupMenu.save()
	advanceFrames(g, 2)
	drawGroupMenuFrame(g) // refresh the label against the just-enabled rule
	grp, _ := g.graph.Group(gid)
	if len(grp.Rules) != 1 || !grp.RulesEnabled {
		t.Fatalf("setup: want 1 enabled rule, got %+v", grp)
	}
	if got, want := ruleToggleLabel(t, g), i18n.T(i18n.KeyGroupRuleOn); got != want {
		t.Fatalf("setup: label = %q, want %q before undo", got, want)
	}

	g.undoManager.Undo()
	if g.groupMenu.IsOpen() {
		t.Fatalf("setup: Import (undo's restore path) must close the group menu")
	}
	grp, ok := g.graph.Group(gid)
	if !ok || len(grp.Rules) != 0 {
		t.Fatalf("setup: undo must restore the group with 0 rules, got ok=%v rules=%+v", ok, grp.Rules)
	}

	g.groupMenu.OpenAt(gid, 100, 100) // reopen post-undo, e.g. via the sidebar chip
	advanceFrames(g, 1)
	if got, want := ruleToggleLabel(t, g), i18n.T(i18n.KeyGroupRuleOff); got != want {
		t.Fatalf("label after undo+reopen = %q, want %q (KeyGroupRuleOff) — must read through to live model, not stale scratch", got, want)
	}
}

// The button is a fixed half-row width, so the longer Enabled/Disabled labels risk real truncation.
func TestGroupMenuRuleToggleLabelFitsAtEveryDensity(t *testing.T) {
	densities := []Density{DensityCompact, DensityComfortable, DensitySpacious}
	for _, d := range densities {
		restore := SetDensityForTest(d)
		g, aID, bID := buildGroupLoopCircuit(t)
		gid, err := g.graph.CreateGroup("", []model.NodeID{aID, bID})
		if err != nil {
			restore()
			t.Fatalf("density=%v: CreateGroup: %v", d, err)
		}
		g.groupMenu.OpenAt(gid, 100, 100)
		advanceFrames(g, 1)

		b, ok := g.groupMenu.btns[gmRuleOnOff]
		if !ok || b == nil {
			restore()
			t.Fatalf("density=%v: ruleonoff button missing", d)
		}
		r, ok := g.groupMenu.rects[gmRuleOnOff]
		if !ok || r.Empty() {
			restore()
			t.Fatalf("density=%v: ruleonoff rect missing/empty", d)
		}
		// Button.Draw's own clip width, in both locales; the Spanish labels are the tighter fit.
		maxW := r.Dx() - 2*SpaceXS
		func() {
			defer i18n.SetLocale(i18n.LocaleEN)
			for _, loc := range i18n.AllLocales() {
				i18n.SetLocale(loc)
				for _, label := range []string{i18n.T(i18n.KeyGroupRuleOn), i18n.T(i18n.KeyGroupRuleOff)} {
					if clipped := clipTextToWidth(label, maxW); clipped != label {
						t.Fatalf("density=%v locale=%v: label %q clips to %q within button width %d (rect=%v) — relabel overflows the ruleonoff button", d, loc, label, clipped, maxW, r)
					}
				}
			}
		}()
		restore()
	}
}

package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// buildGroupLoopCircuitSized mirrors buildGroupLoopCircuit but takes an
// explicit window size and returns a 3-node loop (a -> b -> c -> a) so tests
// can exercise GroupMenu layout at a caller-chosen viewport (e.g. the
// 1280x720 default splitter where the fixed 9-row layout used to overflow
// the grid pane — see TestGroupMenuFitsWithinGridPane).
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

// TestGroupMenuFitsWithinGridPane is the layout-fit regression: GroupMenu's
// panel (and every control inside it) must ALWAYS render entirely inside the
// grid pane, like the node sidebar does. Before the fix, layoutAt computed a
// fixed 9-row height without clamping it against gridH — at common window
// sizes (800x600, and the 1280x720 default splitter where the grid pane is
// well under half the window) the panel extended below the grid pane, where
// the drum view overpaints it, making the Delta/Every-N steppers and Delete
// button invisible and unreachable.
func TestGroupMenuFitsWithinGridPane(t *testing.T) {
	sizes := []struct{ w, h int }{{800, 600}, {1280, 720}}
	// Every control the brief calls out by id, including "save" (only present
	// while provisional — hence OpenAtProvisional below). The old "param"
	// cycle button is gone (task-rule-anchor Part B); the three per-param
	// delta rows (rdpit/rdvol/rddur) replace it.
	controlIDs := []string{
		"delete", "n-", "n+", "save", "close", "ruleonoff",
		"rdpit-", "rdpit+", "rdvol-", "rdvol+", "rddur-", "rddur+",
	}

	// Ruleonoff/Every-N combined row (n- used to render invisible because the
	// pill's full-sentence label overflowed past its left edge and painted
	// over the already-drawn n- button — the rect itself was never missing/
	// zero-width, so plain containment checks above didn't catch it).
	stepPairIDs := []string{"n-", "nval", "n+"}

	for _, sz := range sizes {
		g, aID, bID, cID := buildGroupLoopCircuitSized(t, sz.w, sz.h)
		gid, err := g.graph.CreateGroup("", []model.NodeID{aID, bID, cID})
		if err != nil {
			t.Fatalf("%dx%d: CreateGroup: %v", sz.w, sz.h, err)
		}
		// Match the real post-marquee UI (sceneGroupMenuSetup): a live rule at
		// EveryN=4 so the pill-fit assertion below exercises the exact value
		// the defect screenshots showed truncating.
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

			// Every rect in the combined delta/everyN row must exist, be
			// non-empty, and (for the +/- buttons) be at least
			// sidebarIncBtnW wide.
			rects := make(map[string]image.Rectangle, len(stepPairIDs))
			for _, id := range stepPairIDs {
				r, ok := g.groupMenu.rects[id]
				if !ok || r.Empty() {
					t.Fatalf("%dx%d anchor %v: %q rect missing/empty", sz.w, sz.h, a, id)
				}
				rects[id] = r
			}
			for _, id := range []string{"n-", "n+"} {
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

			// The Every-N pill's actual rendered label must fit inside its
			// own pill rect — this is the assertion that would have caught
			// the defect: the pre-fix label was the full "Every %d rounds"
			// sentence, which measures wider than the pill at every density
			// and overflowed onto the n- button.
			label := g.groupMenu.everyNPillLabel(g.groupMenu.sharedEveryN())
			if tw, pw := StyledTextWidth(label, RoleBody), rects["nval"].Dx(); tw > pw {
				t.Fatalf("%dx%d anchor %v: everyN pill label %q width %d overflows pill width %d", sz.w, sz.h, a, label, tw, pw)
			}
		}
	}
}

// TestGroupMenuRuleEditing pins the independent Enabled/Disabled status bit
// (task-enabled-status brief, Part C): stepRuleDelta/stepRuleEveryN write
// Rules but NEVER flip RulesEnabled — the group stays Disabled purely from
// configuring deltas. setRuleEnabled(false)/(true) toggle ONLY the flag,
// never touching the configured Rules (superseding the old "Off clears
// rules" behavior this test used to pin).
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

// TestGroupMenuTitleShowsGroupName is the spec §4 gap follow-up: the header
// used to always draw the static i18n title regardless of the group's real
// Name. titleText() (read by Draw) must surface the live group name, falling
// back to the i18n title only when the group is unnamed or has vanished.
func TestGroupMenuTitleShowsGroupName(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	// CreateGroup("", ...) auto-names ("Group N"), so drive the empty-Name
	// fallback case explicitly via RenameGroup (which, unlike CreateGroup,
	// performs no empty-string validation) rather than relying on an
	// unreachable-in-practice CreateGroup state.
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

// TestChipReopenCloseDoesNotDelete is task-groupsave brief test 6: a
// committed group (CreateGroup + OpenAt, non-provisional — the sidebar
// group-chip reopen path) must survive a plain Close(); the discard-on-close
// behavior only applies to provisional (unsaved marquee) groups.
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

// TestProvisionalDeleteButtonSilent is task-groupsave brief test 7: pressing
// "Delete group" on a still-provisional group is a silent discard — same as
// closing without Save — never emitting a delete for a group that was never
// committed.
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

// TestProvisionalGroupEmptiedByNodeDeleteNoDoubleDelete is task-groupsave
// brief test 8: deleting the sole member of a provisional group triggers the
// model's auto-delete-on-empty (RemoveNodeFromGroup) which fires the vanish
// path (onGroupsChanged -> groupMenu.Close()) re-entrantly while the group is
// still marked provisional. Close()'s "group still exists" guard must make
// this a no-op rather than a double-delete/panic, and the ONLY undo step
// recorded must be the node deletion itself (the group's own creation/
// discard never emits).
func TestProvisionalGroupEmptiedByNodeDeleteNoDoubleDelete(t *testing.T) {
	g, aID, _ := buildGroupLoopCircuit(t)
	// Marquee over ONE node only: the two nodes in buildGroupLoopCircuit sit
	// only fractions of a pixel apart at the default zoom (adjacent grid
	// cells), so expanding the rect symmetrically would sweep in node b too.
	// Drag from up-and-left of node a to exactly node a's own far (bottom-
	// right) edge — big enough to clear marqueeMinDragPx, but never
	// extending toward node b.
	ax1, ay1, ax2, ay2 := g.nodeScreenRect(g.nodesByID[aID])
	// The press point must be genuinely EMPTY per production's own arming
	// predicates: outside every node sprite, outside any badge, AND outside
	// the guide-snap radius (guideTargetAt) — a shift+press inside the snap
	// annulus arms a link drag, not a marquee. Walk up-left from node a's
	// corner until production agrees the point is empty, so this test stays
	// correct however the snap radius is tuned.
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

// TestImportClosesProvisionalMenuSilently is task-groupsave brief test 9:
// Import must close a still-open provisional GroupMenu without panicking and
// without leaking a phantom group into the freshly imported document.
func TestImportClosesProvisionalMenuSilently(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	// Capture a group-less export BEFORE the marquee creates a provisional
	// group (mirrors TestLegacyFileWithoutGroupsImportsClean).
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

// TestGroupMenuSaveButtonVisibleOnlyWhileProvisional is task-groupsave brief
// test 10: the "save" control exists only while provisional, and is pruned
// (rect + button both absent) once the menu is opened non-provisionally.
// TestGroupMenuSaveButtonVisibleOnlyWhileProvisional used to pin "save"
// absent for a non-provisional (committed) group. Superseded by
// task-save-always (Part 1): Save is now ALWAYS present, greyed out
// (Disabled) when there are no pending edits — see
// TestSaveButtonAlwaysPresent below, which ports this test's provisional-
// presence assertion and adds the new always-present/disabled contract.

// TestSaveButtonAlwaysPresent is task-save-always brief Part 1 test 1: the
// "save" control/button exist for BOTH provisional and committed groups (no
// more provisional-only pruning — see layoutAt). For a committed group: the
// button starts Disabled (no pending edits right after OpenAt, since
// savedRules == the live group's Rules), a rule edit enables it (dirty), and
// Save re-disables it (savedRules resynced to the just-committed state)
// while leaving the menu open.
func TestSaveButtonAlwaysPresent(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	gid, err := g.graph.CreateGroup("", []model.NodeID{aID, bID})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}

	g.groupMenu.OpenAt(gid, 100, 100) // committed (non-provisional)
	drawGroupMenuFrame(g)

	if _, ok := g.groupMenu.rects["save"]; !ok {
		t.Fatalf("save control rect must exist for a committed group")
	}
	b, ok := g.groupMenu.btns["save"]
	if !ok || b == nil {
		t.Fatalf("save button must be wired for a committed group")
	}
	if !b.Disabled {
		t.Fatalf("save button must start Disabled: no pending edits right after OpenAt")
	}

	g.groupMenu.stepRuleDelta(model.GroupParamPitch, 1)
	advanceFrames(g, 2)
	drawGroupMenuFrame(g)
	b, ok = g.groupMenu.btns["save"]
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
	b, ok = g.groupMenu.btns["save"]
	if !ok || b == nil {
		t.Fatalf("save button missing after Save")
	}
	if !b.Disabled {
		t.Fatalf("save button must be Disabled again immediately after Save commits")
	}
}

// TestGroupMenuFitsWithinGridPane (above, unchanged) already pins the
// provisional case's "save" rect presence; this test covers the committed
// (non-provisional) always-present half of the new contract.

// TestCommittedEditsCancelOnClose is task-save-always brief Part 1 test 2: a
// committed group with a live rule (delta 1); opening the menu and stepping
// the delta to 3 previews live in the model (audible preview must keep
// working), but closing WITHOUT Save must silently restore the pre-open
// snapshot — no undo step, no emit.
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

// TestCommittedSaveEmitsOnce is task-save-always brief Part 1 test 3: 3
// separate edits on an open committed group, then Save, must land as exactly
// ONE undo step; undo restores the pre-open state, redo restores the saved
// state.
func TestCommittedSaveEmitsOnce(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	gid := createCommittedGroup(t, g, "", []model.NodeID{aID, bID})
	g.groupMenu.OpenAt(gid, 100, 100)
	undoBefore := len(g.undoManager.undo)

	// Each edit advances its own frame (advanceFrames(g,1) between them) so
	// they land as 3 genuinely SEPARATE Update()-cycle commits — Game.Update
	// wraps each whole frame in an undo beginGroup/endGroup, so 3 edits
	// queued into the SAME frame would coalesce into 1 step regardless of
	// this task's fix, defeating the point of this test (see the brief's
	// "3 separate edits").
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

// TestSaveKeepsMenuOpen is task-save-always brief Part 1 test 4 — ports the
// old "save closes the menu" assumption (TestMarqueeGroupSavedPersists /
// TestGroupMenuE2E_SaveClickCommitsProvisionalGroup, updated separately) for
// the provisional-creation flow: edit, Save — menu stays OPEN, the group
// persists, provisional clears, the save button is Disabled again, and
// exactly 1 undo step (the creation) lands.
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
	b, ok := g.groupMenu.btns["save"]
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

// TestAnchorChangesDoNotDirty is task-save-always brief Part 1 test 5: a
// play-start re-anchor (model.Graph.ReanchorAllGroupRules, fired from the
// real production play-start seam) must NOT dirty an already-open, clean
// committed menu — AnchorIdx is runtime ramp state, ignored by the
// save/dirty comparison (see rulesEqual).
func TestAnchorChangesDoNotDirty(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	gid := createCommittedGroup(t, g, "", []model.NodeID{aID, bID})
	if err := g.graph.SetGroupRules(gid, []model.GroupRule{{Param: model.GroupParamPitch, Delta: 1, EveryN: 1, AnchorIdx: 50}}); err != nil {
		t.Fatalf("SetGroupRules: %v", err)
	}

	g.groupMenu.OpenAt(gid, 100, 100)
	drawGroupMenuFrame(g)
	b, ok := g.groupMenu.btns["save"]
	if !ok || b == nil {
		t.Fatalf("save button missing")
	}
	if !b.Disabled {
		t.Fatalf("setup: menu must start clean (save disabled)")
	}

	// Real play-start seam (transport button path — TriggerPlayPause consumed
	// by Game.Update, mirroring rule_anchor_test.go's production-closest
	// pattern) re-anchors every rule-bearing group's rules.
	g.playFn = func(id string, vol float64, when ...float64) {}
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
	b, ok = g.groupMenu.btns["save"]
	if !ok || b == nil {
		t.Fatalf("save button missing after play-start")
	}
	if !b.Disabled {
		t.Fatalf("play-start re-anchor must not dirty the menu (AnchorIdx must be ignored by the dirty comparison)")
	}
}

// TestStaleMenuHandleCannotAliasReusedID is task-save-always brief Part 2
// test 9: a stale GroupMenu handle (holding a GroupID that has since been
// deleted and reused by a LATER CreateGroup, per lowest-free allocation)
// must never resolve to the new group — the menu closes on vanish
// (onGroupsChanged) and stays closed even once the id is reused.
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

	// External model-API delete (not through the menu) — mirrors an
	// out-of-band deletion (e.g. a different UI surface, undo).
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

// drawGroupMenuFrame runs a real Update()+Draw() cycle so the ruleonoff
// button's rects/label get refreshed against live model state exactly the
// way the real per-frame render loop does (GroupMenu.Draw calls layoutAt,
// which calls ensureButtons — see grid_pane_layers.go's "grid-group-menu"
// layer registration). Used to prove the label syncs from live state without
// requiring a click on the menu itself (e.g. after an undo).
func drawGroupMenuFrame(g *Game) {
	_ = g.Update()
	img := newTrackedImage("test.groupmenu.sync", g.winW, g.winH)
	defer releaseImage(img)
	g.Draw(img)
}

// ruleToggleLabel reads the ruleonoff button's current displayed text.
func ruleToggleLabel(t *testing.T, g *Game) string {
	t.Helper()
	b, ok := g.groupMenu.btns["ruleonoff"]
	if !ok || b == nil {
		t.Fatalf("ruleonoff button missing")
	}
	return b.Text
}

// TestGroupMenuRuleToggleLabelUsesI18nKeys proves the ruleonoff button's
// label always resolves through i18n.T(KeyGroupRuleOn/Off) (the "Enabled"/
// "Disabled" relabel), never a hardcoded English literal — asserted via the
// keys, not the literal strings, so a future locale/copy change can't
// silently desync this test from the catalog.
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

	clickMenuControl(t, g, "rdpit+")
	advanceFrames(g, 2)
	if grp, _ := g.graph.Group(gid); len(grp.Rules) != 1 {
		t.Fatalf("setup: want 1 rule after rdpit+, got %+v", grp.Rules)
	}
	// task-enabled-status brief Part C: a delta alone must NOT flip the
	// label — it stays Disabled until the ruleonoff toggle is clicked.
	if got, want := ruleToggleLabel(t, g), i18n.T(i18n.KeyGroupRuleOff); got != want {
		t.Fatalf("label (delta configured, not enabled) = %q, want %q (KeyGroupRuleOff)", got, want)
	}

	clickMenuControl(t, g, "ruleonoff")
	advanceFrames(g, 2)
	if got, want := ruleToggleLabel(t, g), i18n.T(i18n.KeyGroupRuleOn); got != want {
		t.Fatalf("label (rule enabled) = %q, want %q (KeyGroupRuleOn)", got, want)
	}
}

// TestGroupMenuRuleToggleLabelSyncsOnReopen proves the label always derives
// from the live group's Rules, not menu-local scratch that could go stale
// across a close/reopen: enable+commit a rule, close the menu, reopen it (the
// sidebar-chip-reopen path), and the label must read Enabled immediately —
// not depend on whatever scratch state a fresh GroupMenu-open sequence seeds.
func TestGroupMenuRuleToggleLabelSyncsOnReopen(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	gid, err := g.graph.CreateGroup("", []model.NodeID{aID, bID})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	g.groupMenu.OpenAt(gid, 100, 100)
	g.groupMenu.stepRuleDelta(model.GroupParamPitch, 1)
	// task-enabled-status brief Part C: the delta alone never enables — the
	// toggle is the only path that flips RulesEnabled.
	g.groupMenu.setRuleEnabled(true)
	advanceFrames(g, 2)
	if grp, _ := g.graph.Group(gid); len(grp.Rules) != 1 || !grp.RulesEnabled {
		t.Fatalf("setup: want 1 enabled rule, got %+v", grp)
	}
	// task-save-always (Part 1): a committed group's edits are only a live
	// preview until Save commits them — Close() without Save now silently
	// reverts (restoreIfDirty). Save here so the edit actually persists
	// across the close/reopen this test exercises.
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

// TestGroupMenuRuleToggleLabelSyncsOnUndo proves the label follows live model
// state after an undo, even though the mutation didn't come from a click on
// the menu itself: enable+commit a rule (one undo step), undo (rules gone),
// then reopen the menu — the label must show Disabled, not a stale "Enabled"
// carried over from scratch state.
//
// Note: undo restores state via a full document reimport (undoRestore ->
// Game.Import, undo_seam.go/import.go), and Import unconditionally closes
// the group menu (import.go's "g.groupMenu.Close()" — a pre-existing,
// deliberate replace-not-merge invariant, not something this task changes).
// So "leave the menu open across the undo" (as an initial literal reading of
// the brief's Part 2 test description might suggest) is not reachable given
// that architecture; reopening after undo is the faithful equivalent that
// exercises the same "no stale scratch" concern the brief is guarding
// against — the reopen path is exactly what a real user does next (the
// sidebar badge / re-click), and TestGroupMenuRuleToggleLabelSyncsOnReopen
// above already separately covers the plain reopen-after-close case.
func TestGroupMenuRuleToggleLabelSyncsOnUndo(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	// createCommittedGroup (not the raw g.graph.CreateGroup the sibling tests
	// above use) is required here: it also fires emitGroupCreated, recording
	// the creation as its own undo baseline. A raw CreateGroup call bypasses
	// every emit chokepoint, so the undo manager's committed baseline would
	// still be the PRE-creation (no-group) document; a single Undo() below
	// would then wipe the group entirely instead of just clearing its rule.
	gid := createCommittedGroup(t, g, "", []model.NodeID{aID, bID})
	g.groupMenu.OpenAt(gid, 100, 100)
	g.groupMenu.stepRuleDelta(model.GroupParamPitch, 1)
	// task-enabled-status brief Part C: the delta alone never enables.
	g.groupMenu.setRuleEnabled(true)
	advanceFrames(g, 2)
	// task-save-always (Part 1): the edit is only a live preview until Save
	// commits it as the single undo step this test then undoes — without
	// this, there is no rule-enable undo step at all (the edit never
	// emitted), and the Undo() below would instead undo the group's own
	// creation.
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

// TestGroupMenuRuleToggleLabelFitsAtEveryDensity proves the relabeled
// "Enabled"/"Disabled" strings (longer than the old "On"/"Off") don't get
// clipped by Button.Draw's clipTextToWidth at any density — the ruleonoff
// button's rect is a fixed half-row share of the panel width, not a
// self-shrinking pill, so a longer label is a real truncation risk the old
// "On"/"Off" never exercised.
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

		b, ok := g.groupMenu.btns["ruleonoff"]
		if !ok || b == nil {
			restore()
			t.Fatalf("density=%v: ruleonoff button missing", d)
		}
		r, ok := g.groupMenu.rects["ruleonoff"]
		if !ok || r.Empty() {
			restore()
			t.Fatalf("density=%v: ruleonoff rect missing/empty", d)
		}
		// Mirrors Button.Draw's own clip call (cap.Dx() == r.Dx() — see
		// keycapCapRect, which only adjusts Y) so this test fails exactly
		// when production rendering would actually truncate the label.
		// Checks both locales — "Desactivada"/"Activada" are longer than
		// their English counterparts and are the tighter fit risk.
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

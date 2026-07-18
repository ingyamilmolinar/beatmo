package ui

import (
	"image"
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// ─── Shared helpers ─────────────────────────────────────────────────────────

// almostEqual compares floats with a small epsilon, needed because the
// delta steppers accumulate via float64 arithmetic which isn't guaranteed
// bit-identical to the literal constant.
func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

// clickMenuControl fails the test if the named GroupMenu control's rect is
// missing/empty, else clicks its center through the REAL Game.Update input
// path (gridClick). Per the house rule (functional tests drive Game.Update,
// not isolated method calls), this is the only way the E2E suite below
// exercises GroupMenu's buttons — mirroring the already-established fact
// (zz_repro_groupmenu_click_test.go, superseded by this file) that rects are
// live and clickable without any Draw() call in this harness.
func clickMenuControl(t *testing.T, g *Game, id string) {
	t.Helper()
	r, ok := g.groupMenu.rects[id]
	if !ok || r.Empty() {
		t.Fatalf("clickMenuControl(%q): rect missing/empty (rects=%v)", id, g.groupMenu.rects)
	}
	cx, cy := (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2
	gridClick(g, cx, cy)
}

// ─── A. Every control through real clicks (positive) ───────────────────────

// findRule returns a pointer to the rule for param, or nil.
func findRule(rules []model.GroupRule, param model.GroupParam) *model.GroupRule {
	for i := range rules {
		if rules[i].Param == param {
			return &rules[i]
		}
	}
	return nil
}

// TestGroupMenuE2E_RuleOnOffClickTogglesRule drives the "ruleonoff" button
// through real clicks (task-enabled-status brief, Part C): it toggles the
// group's INDEPENDENT RulesEnabled status bit, never touching the configured
// Rules — a pitch delta is configured via rdpit+ first (deltas alone must
// NOT enable, per Part C), then ruleonoff flips the flag on/off while the
// rule itself stays put throughout.
func TestGroupMenuE2E_RuleOnOffClickTogglesRule(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	gid, err := g.graph.CreateGroup("", []model.NodeID{aID, bID})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	g.groupMenu.OpenAt(gid, 100, 100)
	advanceFrames(g, 2)

	clickMenuControl(t, g, "rdpit+")
	advanceFrames(g, 2)
	grp, _ := g.graph.Group(gid)
	if len(grp.Rules) != 1 {
		t.Fatalf("rdpit+ click must write the rule, got %+v", grp.Rules)
	}
	if grp.RulesEnabled {
		t.Fatalf("rdpit+ click must NOT enable the group, got RulesEnabled=true")
	}

	clickMenuControl(t, g, "ruleonoff")
	advanceFrames(g, 2)
	grp, _ = g.graph.Group(gid)
	if !grp.RulesEnabled {
		t.Fatalf("ruleonoff click must enable the group")
	}
	if len(grp.Rules) != 1 {
		t.Fatalf("ruleonoff click must not touch the configured rule, got %+v", grp.Rules)
	}

	clickMenuControl(t, g, "ruleonoff")
	advanceFrames(g, 2)
	grp, _ = g.graph.Group(gid)
	if grp.RulesEnabled {
		t.Fatalf("second ruleonoff click must disable the group")
	}
	if len(grp.Rules) != 1 || grp.Rules[0].Param != model.GroupParamPitch {
		t.Fatalf("second ruleonoff click must still not touch the configured rule, got %+v", grp.Rules)
	}
}

// TestGroupMenuE2E_DeltaStepClicksApplyIndependentlyPerParam verifies the
// three independent per-param delta steppers (task-rule-anchor Part B,
// replacing the old single delta+param-cycle control): pitch steps by 1.0
// and volume steps by 0.05, and editing one param's delta must not disturb
// another's.
func TestGroupMenuE2E_DeltaStepClicksApplyIndependentlyPerParam(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	gid, err := g.graph.CreateGroup("", []model.NodeID{aID, bID})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	g.groupMenu.OpenAt(gid, 100, 100)
	advanceFrames(g, 2)

	clickMenuControl(t, g, "rdpit+")
	advanceFrames(g, 2)
	grp, _ := g.graph.Group(gid)
	if r := findRule(grp.Rules, model.GroupParamPitch); r == nil || !almostEqual(r.Delta, 1) {
		t.Fatalf("pitch after 1 rdpit+ = %+v, want delta 1", r)
	}

	clickMenuControl(t, g, "rdpit+")
	advanceFrames(g, 2)
	grp, _ = g.graph.Group(gid)
	if r := findRule(grp.Rules, model.GroupParamPitch); r == nil || !almostEqual(r.Delta, 2) {
		t.Fatalf("pitch after 2 rdpit+ = %+v, want delta 2", r)
	}

	clickMenuControl(t, g, "rdvol+")
	advanceFrames(g, 2)
	grp, _ = g.graph.Group(gid)
	if r := findRule(grp.Rules, model.GroupParamVolume); r == nil || !almostEqual(r.Delta, 0.05) {
		t.Fatalf("volume after 1 rdvol+ = %+v, want delta 0.05", r)
	}
	// Pitch rule must be untouched by the volume edit.
	if r := findRule(grp.Rules, model.GroupParamPitch); r == nil || !almostEqual(r.Delta, 2) {
		t.Fatalf("pitch rule disturbed by volume edit: %+v", r)
	}

	clickMenuControl(t, g, "rddur-")
	advanceFrames(g, 2)
	grp, _ = g.graph.Group(gid)
	if r := findRule(grp.Rules, model.GroupParamDuration); r == nil || !almostEqual(r.Delta, -0.05) {
		t.Fatalf("duration after 1 rddur- = %+v, want delta -0.05", r)
	}
	if len(grp.Rules) != 3 {
		t.Fatalf("want exactly 3 rules (one per configured param), got %+v", grp.Rules)
	}
}

// TestGroupMenuE2E_EveryNStepClickFloorsAtOne verifies n+/n- steppers,
// including the EveryN floor at 1.
func TestGroupMenuE2E_EveryNStepClickFloorsAtOne(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	gid, err := g.graph.CreateGroup("", []model.NodeID{aID, bID})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	g.groupMenu.OpenAt(gid, 100, 100)
	advanceFrames(g, 2)

	clickMenuControl(t, g, "rdpit+") // configure a rule, EveryN default 1
	advanceFrames(g, 2)

	clickMenuControl(t, g, "n-") // already at floor
	advanceFrames(g, 2)
	grp, _ := g.graph.Group(gid)
	if grp.Rules[0].EveryN != 1 {
		t.Fatalf("n- at floor = %d, want 1", grp.Rules[0].EveryN)
	}

	clickMenuControl(t, g, "n+")
	advanceFrames(g, 2)
	grp, _ = g.graph.Group(gid)
	if grp.Rules[0].EveryN != 2 {
		t.Fatalf("n+ = %d, want 2", grp.Rules[0].EveryN)
	}

	clickMenuControl(t, g, "n-")
	advanceFrames(g, 2)
	grp, _ = g.graph.Group(gid)
	if grp.Rules[0].EveryN != 1 {
		t.Fatalf("n- back down = %d, want 1", grp.Rules[0].EveryN)
	}
}

// TestGroupMenuE2E_SaveClickCommitsProvisionalGroup: a real marquee creates a
// provisional group; a real "save" click must commit it as exactly 1 undo
// step and keep the menu open (task-save-always Part 1 — Save no longer
// closes the menu).
func TestGroupMenuE2E_SaveClickCommitsProvisionalGroup(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	undoBefore := len(g.undoManager.undo)
	gid := marqueeCreateGroup(t, g, aID, bID)
	advanceFrames(g, 2)

	clickMenuControl(t, g, "save")
	advanceFrames(g, 2)

	if !g.groupMenu.IsOpen() {
		t.Fatalf("Save must keep the menu open")
	}
	if g.groupMenu.provisional {
		t.Fatalf("provisional flag must be cleared after save")
	}
	if _, ok := g.graph.Group(gid); !ok {
		t.Fatalf("saved group must persist")
	}
	if got := len(g.undoManager.undo) - undoBefore; got != 1 {
		t.Fatalf("save must emit exactly 1 undo step, got %d", got)
	}
}

// TestGroupMenuE2E_CloseClickDiscardsProvisionalGroup: a real "close" click
// on a still-provisional (never-saved) group must silently discard it — no
// undo step.
func TestGroupMenuE2E_CloseClickDiscardsProvisionalGroup(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	undoBefore := len(g.undoManager.undo)
	gid := marqueeCreateGroup(t, g, aID, bID)
	advanceFrames(g, 2)

	clickMenuControl(t, g, "close")
	advanceFrames(g, 2)

	if g.groupMenu.IsOpen() {
		t.Fatalf("menu must be closed")
	}
	if _, ok := g.graph.Group(gid); ok {
		t.Fatalf("provisional group must be discarded on close click")
	}
	if got := len(g.undoManager.undo); got != undoBefore {
		t.Fatalf("close-without-save must not add an undo step, before=%d after=%d", undoBefore, got)
	}
}

// TestGroupMenuE2E_DeleteClickRemovesCommittedGroup: a real "delete" click on
// an already-committed (non-provisional) group deletes it and emits exactly
// 1 undo step.
func TestGroupMenuE2E_DeleteClickRemovesCommittedGroup(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	gid, err := g.graph.CreateGroup("", []model.NodeID{aID, bID})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	// Baseline the group's creation into the undo snapshot journal the same
	// way a real Save would (emitGroupCreated) — a direct graph.CreateGroup
	// call is untracked, so without this the journal's "committed" baseline
	// never included the group in the first place, and deleting it would
	// land back on that same untracked baseline (zero diff, no undo entry
	// pushed by design — see UndoManager.commitNow). Baselining first isolates
	// the delete-click's own step, matching how a real committed group
	// reached this state.
	grp, _ := g.graph.Group(gid)
	emitGroupCreated(gid, grp.Name, grp.NodeIDs)
	advanceFrames(g, 2)

	g.groupMenu.OpenAt(gid, 100, 100)
	advanceFrames(g, 2)
	undoBefore := len(g.undoManager.undo)

	clickMenuControl(t, g, "delete")
	advanceFrames(g, 2)

	if g.groupMenu.IsOpen() {
		t.Fatalf("menu must close after delete")
	}
	if _, ok := g.graph.Group(gid); ok {
		t.Fatalf("group must be deleted")
	}
	if got := len(g.undoManager.undo) - undoBefore; got != 1 {
		t.Fatalf("delete on committed group must emit exactly 1 undo step, got %d", got)
	}
}

// TestGroupMenuE2E_ClickInsidePanelNoControlIsConsumed is the negative
// click-through guard: a real click inside the panel bounds but on no
// control (the "rulelabel" half-row, which has no button) must be consumed
// — the menu stays open, no model change, no undo step, and (crucially) no
// node gets created at that grid position, which would happen if the click
// fell through to the grid beneath (see HandleInput's "opaque to z"
// contract).
func TestGroupMenuE2E_ClickInsidePanelNoControlIsConsumed(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	gid, err := g.graph.CreateGroup("", []model.NodeID{aID, bID})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	g.groupMenu.OpenAt(gid, 100, 100)
	advanceFrames(g, 2)

	r, ok := g.groupMenu.rects["rulelabel"]
	if !ok || r.Empty() {
		t.Fatalf("setup: rulelabel rect missing/empty")
	}
	cx, cy := (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2
	pt := image.Pt(cx, cy)
	for id, cr := range g.groupMenu.rects {
		if id == "panel" || id == "title" || id == "rulelabel" {
			continue
		}
		if pt.In(cr) {
			t.Fatalf("setup: chosen point (%d,%d) unexpectedly inside control %q %v", cx, cy, id, cr)
		}
	}

	nodesBefore := len(g.nodes)
	undoBefore := len(g.undoManager.undo)

	gridClick(g, cx, cy)
	advanceFrames(g, 2)

	if !g.groupMenu.IsOpen() {
		t.Fatalf("click on panel whitespace must not close the menu")
	}
	if len(g.nodes) != nodesBefore {
		t.Fatalf("click-through created a node at (%d,%d); want none (panel must be opaque to z)", cx, cy)
	}
	if _, ok := g.graph.Group(gid); !ok {
		t.Fatalf("group must be unaffected by a whitespace click")
	}
	if got := len(g.undoManager.undo); got != undoBefore {
		t.Fatalf("whitespace click must not add an undo step, before=%d after=%d", undoBefore, got)
	}
}

// ─── B. Playback-time edits ─────────────────────────────────────────────────

// TestGroupMenuE2E_PlaybackRuleEditPreservesTriggerParity: enabling a group
// rule via real clicks (two rdpit+ clicks) on a COMMITTED group during
// playback must never alter trigger/audible decisions — mirrors
// parity_group_rule_test.go's TestParityGroupRuleNeverAltersTriggerParity,
// driven through the real click path instead of the model API directly.
func TestGroupMenuE2E_PlaybackRuleEditPreservesTriggerParity(t *testing.T) {
	assertDefaultParityState(t)
	g, aID, bID := buildGroupLoopCircuit(t)
	for i := range g.drum.Rows[0].Steps {
		g.drum.Rows[0].Steps[i] = true
	}
	g.refreshDrumRow()
	baseline := append([]bool(nil), g.drum.Rows[0].Steps...)

	gid, err := g.graph.CreateGroup("", []model.NodeID{aID, bID})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	g.groupMenu.OpenAt(gid, 100, 100) // committed (non-provisional)
	advanceFrames(g, 2)

	g.SetPlaying(true)
	t.Cleanup(func() { stopPlaybackForTest(g) })
	advanceFrames(g, 5)

	clickMenuControl(t, g, "rdpit+") // configure rule (pitch, delta 1, everyN 1)
	advanceFrames(g, 2)
	clickMenuControl(t, g, "rdpit+") // delta 1 -> 2
	advanceFrames(g, 3)
	clickMenuControl(t, g, "ruleonoff") // deltas alone never enable — flip it on
	advanceFrames(g, 3)

	grp, ok := g.graph.Group(gid)
	if !ok || len(grp.Rules) != 1 || grp.Rules[0].Param != model.GroupParamPitch || !almostEqual(grp.Rules[0].Delta, 2) {
		t.Fatalf("want pitch rule delta=2 after two rdpit+ clicks, got %+v ok=%v", grp, ok)
	}
	if !grp.RulesEnabled {
		t.Fatalf("want RulesEnabled=true after the ruleonoff click, got %+v", grp)
	}

	// Trigger parity must be UNCHANGED by the rule.
	for i, on := range baseline {
		abs := g.drum.Offset + i
		if got := g.drum.Rows[0].Steps[i]; got != on {
			t.Fatalf("rule click changed rendered Steps at i=%d: got=%v want=%v", i, got, on)
		}
		if got := g.engine.Predictor.VisibleAt(0, abs); got != on {
			t.Fatalf("rule click changed predictor VisibleAt at abs=%d: got=%v want=%v", abs, got, on)
		}
	}

	loopStart := g.loopStartByRow[0]
	loopLen := len(g.beatInfosByRow[0]) - loopStart
	if !g.isLoopByRow[0] || loopLen <= 0 {
		t.Fatalf("expected loop row, isLoop=%v loopLen=%d", g.isLoopByRow[0], loopLen)
	}
	infoAt := func(idx int) model.BeatInfo { return g.beatInfoAtRow(0, idx) }
	for round := 0; round < 3; round++ {
		idx := loopStart + round*loopLen
		info := infoAt(idx)
		_, pitch, _ := g.evalNodeParamsOnly(0, idx, info)
		want := float64(round) * 2
		if pitch != want {
			t.Fatalf("round %d pitch = %v, want %v", round, pitch, want)
		}
		if !g.nodeTriggered(0, idx, info) {
			t.Fatalf("round %d: node must still trigger despite the pitch rule", round)
		}
		if want := g.engine.Predictor.VisibleAt(0, idx); !want {
			t.Fatalf("round %d: predictor must still show visible/firing despite the rule", round)
		}
	}
}

// TestGroupMenuE2E_PlaybackMembershipRemovalRevertsBaseParams: removing a
// node from a ruled group during playback (via the model API, per the brief)
// must revert that node's effective pitch to base at future indices, while
// the remaining member keeps the rule's effect.
func TestGroupMenuE2E_PlaybackMembershipRemovalRevertsBaseParams(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	gid, err := g.graph.CreateGroup("", []model.NodeID{aID, bID})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	if err := g.graph.SetGroupRules(gid, []model.GroupRule{
		{Param: model.GroupParamPitch, Delta: 2, EveryN: 1},
	}); err != nil {
		t.Fatalf("SetGroupRules: %v", err)
	}
	if err := g.graph.SetGroupRulesEnabled(gid, true); err != nil {
		t.Fatalf("SetGroupRulesEnabled: %v", err)
	}
	g.refreshDrumRow()

	g.SetPlaying(true)
	t.Cleanup(func() { stopPlaybackForTest(g) })
	advanceFrames(g, 5)

	loopStart := g.loopStartByRow[0]
	loopLen := len(g.beatInfosByRow[0]) - loopStart
	if !g.isLoopByRow[0] || loopLen <= 0 {
		t.Fatalf("expected loop row, isLoop=%v loopLen=%d", g.isLoopByRow[0], loopLen)
	}
	infoAt := func(idx int) model.BeatInfo { return g.beatInfoAtRow(0, idx) }
	findIdx := func(id model.NodeID) int {
		for i := 0; i < loopLen; i++ {
			if infoAt(loopStart+i).NodeID == id {
				return loopStart + i
			}
		}
		t.Fatalf("node %d not present in loop", id)
		return -1
	}
	idxA, idxB := findIdx(aID), findIdx(bID)
	const round = 2
	idxA2, idxB2 := idxA+round*loopLen, idxB+round*loopLen

	_, pitchA, _ := g.evalNodeParamsOnly(0, idxA2, infoAt(idxA2))
	_, pitchB, _ := g.evalNodeParamsOnly(0, idxB2, infoAt(idxB2))
	if pitchA != 4 || pitchB != 4 {
		t.Fatalf("setup: round-2 pitch A=%v B=%v, want both 4", pitchA, pitchB)
	}

	if err := g.graph.RemoveNodeFromGroup(gid, aID); err != nil {
		t.Fatalf("RemoveNodeFromGroup: %v", err)
	}
	advanceFrames(g, 2)

	_, pitchA, _ = g.evalNodeParamsOnly(0, idxA2, infoAt(idxA2))
	_, pitchB, _ = g.evalNodeParamsOnly(0, idxB2, infoAt(idxB2))
	if pitchA != 0 {
		t.Fatalf("removed member A pitch = %v, want back to base 0", pitchA)
	}
	if pitchB != 4 {
		t.Fatalf("remaining member B pitch = %v, want unchanged 4", pitchB)
	}
}

// TestGroupMenuE2E_PlaybackDeleteClickRevertsAllMembersToBase: a real
// "delete" click on a ruled, committed group during playback must revert
// every member's effective pitch to base at future indices.
func TestGroupMenuE2E_PlaybackDeleteClickRevertsAllMembersToBase(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	gid, err := g.graph.CreateGroup("", []model.NodeID{aID, bID})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	if err := g.graph.SetGroupRules(gid, []model.GroupRule{
		{Param: model.GroupParamPitch, Delta: 2, EveryN: 1},
	}); err != nil {
		t.Fatalf("SetGroupRules: %v", err)
	}
	if err := g.graph.SetGroupRulesEnabled(gid, true); err != nil {
		t.Fatalf("SetGroupRulesEnabled: %v", err)
	}
	g.groupMenu.OpenAt(gid, 100, 100)
	advanceFrames(g, 2)

	g.SetPlaying(true)
	t.Cleanup(func() { stopPlaybackForTest(g) })
	advanceFrames(g, 5)

	loopStart := g.loopStartByRow[0]
	loopLen := len(g.beatInfosByRow[0]) - loopStart
	if !g.isLoopByRow[0] || loopLen <= 0 {
		t.Fatalf("expected loop row, isLoop=%v loopLen=%d", g.isLoopByRow[0], loopLen)
	}
	infoAt := func(idx int) model.BeatInfo { return g.beatInfoAtRow(0, idx) }
	findIdx := func(id model.NodeID) int {
		for i := 0; i < loopLen; i++ {
			if infoAt(loopStart+i).NodeID == id {
				return loopStart + i
			}
		}
		t.Fatalf("node %d not present in loop", id)
		return -1
	}
	idxA, idxB := findIdx(aID), findIdx(bID)
	const round = 2
	idxA2, idxB2 := idxA+round*loopLen, idxB+round*loopLen

	_, pitchA, _ := g.evalNodeParamsOnly(0, idxA2, infoAt(idxA2))
	_, pitchB, _ := g.evalNodeParamsOnly(0, idxB2, infoAt(idxB2))
	if pitchA != 4 || pitchB != 4 {
		t.Fatalf("setup: round-2 pitch A=%v B=%v, want both 4", pitchA, pitchB)
	}

	clickMenuControl(t, g, "delete")
	advanceFrames(g, 3)

	if g.groupMenu.IsOpen() {
		t.Fatalf("menu must close after delete")
	}
	if _, ok := g.graph.Group(gid); ok {
		t.Fatalf("group must be gone")
	}

	_, pitchA, _ = g.evalNodeParamsOnly(0, idxA2, infoAt(idxA2))
	_, pitchB, _ = g.evalNodeParamsOnly(0, idxB2, infoAt(idxB2))
	if pitchA != 0 || pitchB != 0 {
		t.Fatalf("post-delete pitch A=%v B=%v, want both back to base 0", pitchA, pitchB)
	}
}

// TestGroupMenuE2E_PlaybackSaveViaRealMarquee: a real marquee-created group
// saved via a real click while playback is running must not panic, must emit
// exactly 1 undo step, and playback must still be running afterward.
func TestGroupMenuE2E_PlaybackSaveViaRealMarquee(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	g.SetPlaying(true)
	t.Cleanup(func() { stopPlaybackForTest(g) })
	advanceFrames(g, 5)

	undoBefore := len(g.undoManager.undo)
	gid := marqueeCreateGroup(t, g, aID, bID)
	if !g.Playing() {
		t.Fatalf("marquee-created group must not stop playback")
	}
	advanceFrames(g, 2)

	clickMenuControl(t, g, "save")
	advanceFrames(g, 3)

	if !g.groupMenu.IsOpen() {
		t.Fatalf("Save must keep the menu open")
	}
	if _, ok := g.graph.Group(gid); !ok {
		t.Fatalf("saved group must persist")
	}
	if got := len(g.undoManager.undo) - undoBefore; got != 1 {
		t.Fatalf("save must emit exactly 1 undo step, got %d", got)
	}
	if !g.Playing() {
		t.Fatalf("playback must continue through save")
	}
}

// ─── C. Negative / API scenarios ────────────────────────────────────────────

// TestGroupMenuE2E_ClickAfterAllMembersDeletedNoPanic: once a group's sole
// member is deleted, the model auto-deletes the (now-empty) group and the
// menu closes. A subsequent plain click at the panel's former screen
// location must not panic, must not reopen the menu, and must not
// resurrect the group.
func TestGroupMenuE2E_ClickAfterAllMembersDeletedNoPanic(t *testing.T) {
	g, aID, _ := buildGroupLoopCircuit(t)
	gid, err := g.graph.CreateGroup("", []model.NodeID{aID})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	g.groupMenu.OpenAt(gid, 100, 100)
	advanceFrames(g, 2)
	if !g.groupMenu.IsOpen() {
		t.Fatalf("setup: menu must be open")
	}
	panel := g.groupMenu.panel
	px, py := (panel.Min.X+panel.Max.X)/2, (panel.Min.Y+panel.Max.Y)/2

	g.deleteNode(g.nodesByID[aID])
	advanceFrames(g, 2)

	if g.groupMenu.IsOpen() {
		t.Fatalf("menu must close once the sole member is deleted")
	}
	if _, ok := g.graph.Group(gid); ok {
		t.Fatalf("group must be gone")
	}

	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("click at former panel location panicked: %v", r)
			}
		}()
		gridClick(g, px, py)
	}()
	advanceFrames(g, 2)

	if g.groupMenu.IsOpen() {
		t.Fatalf("group menu must not reopen from a plain click")
	}
	if _, ok := g.graph.Group(gid); ok {
		t.Fatalf("deleted group must not resurrect")
	}
}

// TestGroupMenuE2E_SetRuleEnabledOnVanishedGroupIsNoop: deleting the group
// out from under an open menu (bypassing the menu's own delete path) closes
// the menu synchronously via the model's group-changed hook, but leaves
// m.groupID pointing at the vanished id. Calling setRuleEnabled(true) after
// that must be a no-op — no panic, no rule resurrection, no group
// recreation — never duplicate core/model's own negative-path coverage.
func TestGroupMenuE2E_SetRuleEnabledOnVanishedGroupIsNoop(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	gid, err := g.graph.CreateGroup("", []model.NodeID{aID, bID})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	g.groupMenu.OpenAt(gid, 100, 100)
	advanceFrames(g, 2)

	if err := g.graph.DeleteGroup(gid); err != nil {
		t.Fatalf("DeleteGroup: %v", err)
	}
	if g.groupMenu.IsOpen() {
		t.Fatalf("setup: menu must have closed synchronously via the group-changed hook")
	}

	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("setRuleEnabled on a vanished group panicked: %v", r)
			}
		}()
		g.groupMenu.setRuleEnabled(true)
	}()
	advanceFrames(g, 2)

	if _, ok := g.graph.Group(gid); ok {
		t.Fatalf("vanished group must not be resurrected by setRuleEnabled")
	}
	if g.groupMenu.IsOpen() {
		t.Fatalf("menu must not reopen")
	}
}

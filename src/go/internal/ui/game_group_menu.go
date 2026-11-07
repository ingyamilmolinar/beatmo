package ui

import (
	"fmt"
	"image"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// A compact anchored popup with no scroll, so one fixed width suffices.
const groupMenuW = 300

// Edits ONE group's rule set through the model API; per-node editing lives in the node sidebar.
type GroupMenu struct {
	game    *Game
	open    bool
	groupID model.GroupID
	anchorX int
	anchorY int
	panel   image.Rectangle
	rects   map[gmID]image.Rectangle
	btns    map[gmID]*Button
	// Same contract as NodeSidebar.drag: zero value passes placement through, a drag pins the card.
	drag PopupDragHandle
	// Scratch seeds for params the group has no rule for yet; the live group's Rules are always the
	// authority. Reseeded on every open, never per frame.
	ruleDeltaVol float64
	ruleDeltaPit float64
	ruleDeltaDur float64
	ruleEveryN   int
	// The open-time snapshot dirty() compares against (order-insensitive, ignoring AnchorIdx). Reset
	// on every save; restored silently into the model when the menu closes dirty without Save.
	savedRules []model.GroupRule
	// Toggling the status is a pending edit like a delta edit: it dirties Save and reverts on close.
	savedEnabled bool
	// Truncated, not reallocated, every layoutAt.
	memberRectsScratch []image.Rectangle
	// A marquee-created group not yet saved. Edits mutate the model live but emit no undo step; any
	// Close() while provisional deletes it silently, and Save emits the one creation step.
	provisional bool
}

func NewGroupMenu(g *Game) *GroupMenu {
	return &GroupMenu{
		game:       g,
		rects:      map[gmID]image.Rectangle{},
		btns:       map[gmID]*Button{},
		ruleEveryN: 1,
	}
}

// The group is treated as already committed.
func (m *GroupMenu) OpenAt(id model.GroupID, x, y int) {
	m.openAt(id, x, y, false)
}

func (m *GroupMenu) OpenAtProvisional(id model.GroupID, x, y int) {
	m.openAt(id, x, y, true)
}

// provisional is set BEFORE layoutAt so the first layout already has (or omits) the save control.
func (m *GroupMenu) openAt(id model.GroupID, x, y int, provisional bool) {
	if _, ok := m.game.graph.Group(id); !ok {
		return
	}
	// A second open while an unsaved provisional menu is up would leak the first group uncommitted
	// and menuless; discard it through the same path Close() uses.
	if m.open && m.provisional && m.groupID != id {
		m.discardProvisional()
	}
	m.open = true
	m.provisional = provisional
	m.drag.Reset()
	m.groupID = id
	// Missing params default to 0 (Off); EveryN to 1 when the group has no rules.
	m.ruleDeltaVol, m.ruleDeltaPit, m.ruleDeltaDur = 0, 0, 0
	m.ruleEveryN = 1
	m.savedRules = nil
	m.savedEnabled = false
	if grp, ok := m.game.graph.Group(id); ok {
		for _, r := range grp.Rules {
			switch r.Param {
			case model.GroupParamVolume:
				m.ruleDeltaVol = r.Delta
			case model.GroupParamDuration:
				m.ruleDeltaDur = r.Delta
			case model.GroupParamPitch:
				m.ruleDeltaPit = r.Delta
			}
			m.ruleEveryN = r.EveryN // every rule shares EveryN by construction
		}
		// grp.Rules is already a deep copy, so no aliasing with the live group.
		m.savedRules = grp.Rules
		m.savedEnabled = grp.RulesEnabled
	}
	m.anchorX, m.anchorY = x, y
	m.layoutAt(x, y)
	m.game.dispatcherDirty = true
}

// No emit, no undo step: the creation was never committed. Idempotent, because the model's
// group-changed hook re-enters Close() while DeleteGroup unwinds. Shared by Close() and openAt().
func (m *GroupMenu) discardProvisional() {
	if !m.provisional {
		return
	}
	if _, ok := m.game.graph.Group(m.groupID); ok {
		_ = m.game.graph.DeleteGroup(m.groupID) // silent: ignore ErrGroupNotFound races
	}
	m.provisional = false
}

// Closing without Save must leave zero trace: revert silently (no emit, no undo step), keeping
// the snapshotted AnchorIdx so ramps are not perturbed.
func (m *GroupMenu) restoreIfDirty() {
	if !m.dirty() {
		return
	}
	id := m.groupID
	if _, ok := m.game.graph.Group(id); !ok {
		return
	}
	if len(m.savedRules) == 0 {
		_ = m.game.graph.ClearGroupRules(id)
	} else {
		_ = m.game.graph.SetGroupRules(id, m.savedRules)
	}
	_ = m.game.graph.SetGroupRulesEnabled(id, m.savedEnabled)
}

// m.open is cleared BEFORE the discard/restore runs: dirty() → group() calls Close() when the
// group vanished, which would otherwise recurse forever.
func (m *GroupMenu) Close() {
	if !m.open {
		return
	}
	wasProvisional := m.provisional
	m.open = false
	if wasProvisional {
		m.discardProvisional()
	} else {
		m.restoreIfDirty()
	}
	m.btns = map[gmID]*Button{}
	m.rects = map[gmID]image.Rectangle{}
	m.drag.Reset()
	m.game.dispatcherDirty = true
}

func (m *GroupMenu) IsOpen() bool           { return m.open }
func (m *GroupMenu) GroupID() model.GroupID { return m.groupID }
func (m *GroupMenu) Hit(x, y int) bool      { return m.open && image.Pt(x, y).In(m.panel) }

// ─── InputHandler interface ────────────────────────────────────────────────

func (m *GroupMenu) InputBounds() image.Rectangle { return m.panel }
func (m *GroupMenu) ZIndex() int                  { return 210 }

func (m *GroupMenu) HandleWheel(_, _, _ int) InputResult { return InputIgnored }

func (m *GroupMenu) Capturing() bool { return m.drag.Dragging() }

// HandleInput processes mouse/touch input within the menu panel.
func (m *GroupMenu) HandleInput(x, y int, pressed bool) InputResult {
	if !m.open {
		return InputIgnored
	}
	// Re-laid against the live model before hit-testing.
	m.layoutAt(m.anchorX, m.anchorY)

	// InputCaptured routes later frames straight here, so the drag survives the cursor leaving the card.
	gridPaneR := image.Rect(0, 0, m.game.split.GridW(m.game.winW), m.game.split.GridH(m.game.winH))
	if m.drag.Dragging() {
		if pressed {
			m.drag.MoveTo(image.Pt(x, y), gridPaneR, m.game.gridTopButtonsReservedRect(),
				image.Pt(m.panel.Dx(), m.panel.Dy()))
			m.layoutAt(m.anchorX, m.anchorY)
		} else {
			m.drag.End()
		}
		return InputCaptured
	}
	if pressed {
		if hit := m.drag.HitRect(m.panel); !hit.Empty() && image.Pt(x, y).In(hit) {
			m.drag.Begin(image.Pt(x, y), m.panel)
			return InputCaptured
		}
	}

	pt := image.Pt(x, y)
	if !pt.In(m.panel) {
		// Defensive: the dispatcher only sends points inside the panel.
		if pressed {
			m.Close()
		}
		return InputIgnored
	}

	order := []gmID{
		gmSave, gmClose, gmRuleOnOff,
		gmMinus(gmStepEveryN), gmPlus(gmStepEveryN), gmMinus(gmStepPitch), gmPlus(gmStepPitch),
		gmMinus(gmStepVolume), gmPlus(gmStepVolume), gmMinus(gmStepDuration), gmPlus(gmStepDuration),
	}
	// Before delete, so the click order matches the visual stacking.
	for _, cand := range m.mergeChipCandidates() {
		order = append(order, gmMergeChip(cand.ID))
	}
	order = append(order, gmDelete)
	for _, id := range order {
		btn, ok := m.btns[id]
		if !ok {
			continue
		}
		r, ok := m.rects[id]
		if !ok || r.Empty() {
			continue
		}
		btn.SetRect(r)
		if btn.HandleInputResult(x, y, pressed) != InputIgnored {
			return InputConsumed
		}
	}
	// Opaque to z: a press on no control is still consumed.
	return InputConsumed
}

// Closes the menu if the group vanished.
func (m *GroupMenu) group() (model.NodeGroup, bool) {
	grp, ok := m.game.graph.Group(m.groupID)
	if !ok && m.open {
		m.Close()
	}
	return grp, ok
}

func (m *GroupMenu) titleText() string {
	grp, ok := m.group()
	if !ok || grp.Name == "" {
		return i18n.T(i18n.KeyGroupMenuTitle)
	}
	return grp.Name
}

// false: synthesized from scratch, the write seed and the pill value.
func (m *GroupMenu) ruleFor(param model.GroupParam) (model.GroupRule, bool) {
	if grp, ok := m.group(); ok {
		for _, r := range grp.Rules {
			if r.Param == param {
				return r, true
			}
		}
	}
	return model.GroupRule{Param: param, Delta: m.scratchDelta(param), EveryN: m.ruleEveryN}, false
}

func (m *GroupMenu) scratchDelta(param model.GroupParam) float64 {
	switch param {
	case model.GroupParamVolume:
		return m.ruleDeltaVol
	case model.GroupParamDuration:
		return m.ruleDeltaDur
	default:
		return m.ruleDeltaPit
	}
}

// setScratchDelta writes back a per-param scratch delta.
func (m *GroupMenu) setScratchDelta(param model.GroupParam, v float64) {
	switch param {
	case model.GroupParamVolume:
		m.ruleDeltaVol = v
	case model.GroupParamDuration:
		m.ruleDeltaDur = v
	default:
		m.ruleDeltaPit = v
	}
}

// Ignores AnchorIdx: it is runtime ramp state re-stamped on play-start, never a document change,
// so it must not make an open menu look dirty.
func rulesEqual(a, b []model.GroupRule) bool {
	if len(a) != len(b) {
		return false
	}
	am := make(map[model.GroupParam]model.GroupRule, len(a))
	for _, r := range a {
		am[r.Param] = r
	}
	for _, r := range b {
		ar, ok := am[r.Param]
		if !ok || ar.Delta != r.Delta || ar.EveryN != r.EveryN {
			return false
		}
	}
	return true
}

// Always true while provisional. Drives Save's Disabled state and whether Close() restores.
func (m *GroupMenu) dirty() bool {
	if m.provisional {
		return true
	}
	grp, ok := m.group()
	if !ok {
		return false
	}
	return !rulesEqual(grp.Rules, m.savedRules) || grp.RulesEnabled != m.savedEnabled
}

// Deliberately not len(grp.Rules) > 0: a group can carry deltas while disabled.
func (m *GroupMenu) rulesEnabled() bool {
	grp, ok := m.group()
	return ok && grp.RulesEnabled
}

func (m *GroupMenu) sharedEveryN() int {
	if grp, ok := m.group(); ok && len(grp.Rules) > 0 {
		return grp.Rules[0].EveryN
	}
	return m.ruleEveryN
}

// Fixed param order for byte-stable export; all-zero deltas clear the rules, never RulesEnabled.
// An existing rule keeps its AnchorIdx so a live tweak continues its ramp; only a new rule gets
// a fresh anchor. Mutates the model live but never emits: only save() does.
func (m *GroupMenu) writeRules() {
	g := m.game
	id := m.groupID
	order := [...]model.GroupParam{model.GroupParamPitch, model.GroupParamVolume, model.GroupParamDuration}
	deltas := [...]float64{m.ruleDeltaPit, m.ruleDeltaVol, m.ruleDeltaDur}
	everyN := m.ruleEveryN
	g.enqueueUI(func() {
		existingAnchor := map[model.GroupParam]int{}
		if grp, ok := g.graph.Group(id); ok {
			for _, r := range grp.Rules {
				existingAnchor[r.Param] = r.AnchorIdx
			}
		}
		freshAnchor := g.ruleAnchorIdxNow()
		var rules []model.GroupRule
		for i, param := range order {
			if deltas[i] == 0 {
				continue
			}
			anchor := freshAnchor
			if a, ok := existingAnchor[param]; ok {
				anchor = a
			}
			rules = append(rules, model.GroupRule{Param: param, Delta: deltas[i], EveryN: everyN, AnchorIdx: anchor})
		}
		if len(rules) == 0 {
			_ = g.graph.ClearGroupRules(id)
			return
		}
		_ = g.graph.SetGroupRules(id, rules)
	})
}

// Never touches Rules: disabling silences the ramp without discarding the deltas. Live (the hook
// re-predicts the next trigger), but only save() emits.
func (m *GroupMenu) setRuleEnabled(on bool) {
	g := m.game
	id := m.groupID
	g.enqueueUI(func() {
		_ = g.graph.SetGroupRulesEnabled(id, on)
	})
}

// Never touches RulesEnabled: a delta edit must not implicitly enable a group.
func (m *GroupMenu) stepRuleDelta(param model.GroupParam, dir int) {
	step := 1.0
	if param != model.GroupParamPitch {
		step = 0.05
	}
	m.setScratchDelta(param, m.scratchDelta(param)+float64(dir)*step)
	m.writeRules()
}

// stepRuleEveryN nudges the shared EveryN (floored at 1) and writes.
func (m *GroupMenu) stepRuleEveryN(dir int) {
	m.ruleEveryN += dir
	if m.ruleEveryN < 1 {
		m.ruleEveryN = 1
	}
	m.writeRules()
}

// Provisional: emit the one creation step and clear provisional. Committed and dirty: emit one
// change step. The menu stays open and Save greys out; wasProvisional is captured before the closure runs.
func (m *GroupMenu) save() {
	g := m.game
	id := m.groupID
	wasProvisional := m.provisional
	g.enqueueUI(func() {
		grp, ok := g.graph.Group(id)
		if !ok {
			return
		}
		if wasProvisional {
			emitGroupCreated(id, grp.Name, grp.NodeIDs)
			m.provisional = false
		} else if !rulesEqual(grp.Rules, m.savedRules) || grp.RulesEnabled != m.savedEnabled {
			emitGroupChanged(id)
		}
		m.savedRules = grp.Rules
		m.savedEnabled = grp.RulesEnabled
	})
}

func (m *GroupMenu) deleteGroup() {
	g := m.game
	id := m.groupID
	g.enqueueUI(func() {
		if m.provisional {
			// Through Close(): every provisional discard must go through discardProvisional.
			m.Close()
			return
		}
		if err := g.graph.DeleteGroup(id); err == nil {
			emitGroupDeleted(id)
		}
		m.Close()
	})
}

// ─── Merge chips ("Add to group") ──────────────────────────────────────────

// No scrolling in this popup.
const mergeChipCandidateCap = 4

const mergeChipCols = 2

// Skips groups that already hold the whole selection (merging would be a no-op); nil unless
// provisional, the only time chips show.
func (m *GroupMenu) mergeChipCandidates() []model.NodeGroup {
	if !m.provisional {
		return nil
	}
	grp, ok := m.game.graph.Group(m.groupID)
	if !ok {
		return nil
	}
	selected := make(map[model.NodeID]bool, len(grp.NodeIDs))
	for _, id := range grp.NodeIDs {
		selected[id] = true
	}
	var out []model.NodeGroup
	for _, other := range m.game.graph.AllGroups() { // AllGroups is ascending-GroupID sorted.
		if other.ID == m.groupID {
			continue
		}
		otherSet := make(map[model.NodeID]bool, len(other.NodeIDs))
		for _, id := range other.NodeIDs {
			otherSet[id] = true
		}
		hasAll := true
		for id := range selected {
			if !otherSet[id] {
				hasAll = false
				break
			}
		}
		if hasAll {
			continue // no-op merge: gid already contains every selected member.
		}
		out = append(out, other)
		if len(out) >= mergeChipCandidateCap {
			break
		}
	}
	return out
}

// Captures the members BEFORE discarding the provisional group (afterwards it is gone), adds them
// to gid in one undo step and reopens gid committed. The re-entrant Close from the discard is a no-op.
func (m *GroupMenu) mergeInto(gid model.GroupID) {
	g := m.game
	provisionalID := m.groupID
	anchorX, anchorY := m.anchorX, m.anchorY
	g.enqueueUI(func() {
		grp, ok := g.graph.Group(provisionalID)
		if !ok {
			return
		}
		members := append([]model.NodeID(nil), grp.NodeIDs...)
		m.discardProvisional()
		beginUndoGroup("merge into group")
		added := false
		for _, id := range members {
			if err := g.graph.AddNodeToGroup(gid, id); err != nil {
				g.logger.Debugf("[groupmenu] mergeInto AddNodeToGroup(%d, %d): %v", gid, id, err)
			} else {
				added = true
			}
		}
		// Emit only if the target changed: it may have been deleted since the chips were laid out.
		if added {
			emitGroupChanged(gid)
		}
		endUndoGroup()
		m.OpenAt(gid, anchorX, anchorY)
	})
}

// ─── Layout ────────────────────────────────────────────────────────────────

const groupMenuFullRows = 5

// The "Rule" label is a half-height row, which lets the panel fit the grid pane at normal sizes.
const groupMenuHalfRows = 1

// Shrinks the gap, then the row, to the 24/3 px floors, then below them if it must: the panel
// never overflows the grid pane. extra* fold in the merge-chip rows.
func computeGroupMenuRowSizing(maxH int, extraHalfRows, extraFullRows int) (rowH, gap int) {
	rowH, gap = sidebarBtnH, sidebarGap
	contentH := func(rh, gp int) int {
		halfH := rh / 2
		return sidebarPad*2 + sidebarHeaderH + gp +
			(groupMenuHalfRows+extraHalfRows)*(halfH+gp) + (groupMenuFullRows+extraFullRows)*(rh+gp)
	}
	for contentH(rowH, gap) > maxH && (rowH > 24 || gap > 3) {
		switch {
		case gap > 3:
			gap--
		case rowH > 24:
			rowH--
		}
	}
	for contentH(rowH, gap) > maxH && (rowH > 0 || gap > 0) {
		if rowH > 0 {
			rowH--
		}
		if gap > 0 {
			gap--
		}
	}
	return rowH, gap
}

// Clamped inside the grid pane in position and size; shares the sidebar's pad/header rhythm.
func (m *GroupMenu) layoutAt(x, y int) {
	g := m.game
	m.rects = map[gmID]image.Rectangle{}

	gridW := g.split.GridW(g.winW)
	gridH := g.split.GridH(g.winH)

	w := groupMenuW
	if w > gridW {
		w = gridW
	}
	if w < 0 {
		w = 0
	}

	// Before row sizing, so the merge section's rows join the fit pass.
	mergeCandidates := m.mergeChipCandidates()
	extraHalfRows, extraChipRows := 0, 0
	if n := len(mergeCandidates); n > 0 {
		extraHalfRows = 1
		extraChipRows = (n + mergeChipCols - 1) / mergeChipCols
	}

	rowH, gap := computeGroupMenuRowSizing(gridH, extraHalfRows, extraChipRows)
	halfH := rowH / 2

	h := sidebarPad*2 + sidebarHeaderH + gap +
		(groupMenuHalfRows+extraHalfRows)*(halfH+gap) + (groupMenuFullRows+extraChipRows)*(rowH+gap)

	// Candidates adjacent to the anchor, re-checked for member overlap AFTER the gear adjustment, so
	// the panel never lands on a member. Re-runs every frame, tracking members as the camera moves.
	reserved := g.gridHelpReservedRect()
	members := []image.Rectangle(nil)
	if grp, ok := g.graph.Group(m.groupID); ok {
		m.memberRectsScratch = g.groupMemberScreenRects(grp.NodeIDs, m.memberRectsScratch)
		members = m.memberRectsScratch
	}
	// While pinned, no auto-placement: a member drifting under the card must not shove it off the
	// user's drop point.
	gridPaneRect := image.Rect(0, 0, gridW, gridH)
	panel := image.Rect(0, 0, w, h)
	if !m.drag.Pinned() {
		panel = placeGroupMenu(image.Pt(x, y), image.Pt(w, h), members, reserved, gridW, gridH)
	}
	panel = m.drag.Place(panel, gridPaneRect, g.gridTopButtonsReservedRect())
	m.panel = panel
	m.rects[gmPanel] = panel
	m.rects[gmGrip] = m.drag.GripRect(panel)

	headerR := image.Rect(panel.Min.X+sidebarPad, panel.Min.Y+sidebarPad, panel.Max.X-sidebarPad, panel.Min.Y+sidebarPad+sidebarHeaderH)
	titleR := headerR
	if gr := m.rects[gmGrip]; !gr.Empty() {
		if x := gr.Max.X + sidebarGap; x > titleR.Min.X {
			titleR.Min.X = x
		}
	}
	m.rects[gmTitle] = titleR
	closeSize := sidebarBtnH
	// Desktop only; mobile dismisses on an outside tap, and Save takes the freed space.
	if desktopCloseVisible() {
		m.rects[gmClose] = image.Rect(headerR.Max.X-closeSize, headerR.Min.Y, headerR.Max.X, headerR.Min.Y+closeSize)
	} else {
		delete(m.rects, gmClose)
	}

	// Always present, disabled when there is nothing to commit, rather than pruned.
	label := i18n.T(i18n.KeyGroupSave)
	textW := StyledTextWidth(label, RoleBody)
	saveW := textW + 4*SpaceXS
	if minW := closeSize * 2; saveW < minW {
		saveW = minW
	}
	saveX2 := headerR.Max.X
	if r, ok := m.rects[gmClose]; ok {
		saveX2 = r.Min.X - sidebarGap
	}
	saveX1 := saveX2 - saveW
	if saveX1 < headerR.Min.X {
		saveX1 = headerR.Min.X
	}
	m.rects[gmSave] = image.Rect(saveX1, headerR.Min.Y, saveX2, headerR.Min.Y+closeSize)

	yy := headerR.Max.Y + gap
	rowRect := func(id gmID) image.Rectangle {
		r := image.Rect(panel.Min.X+sidebarPad, yy, panel.Max.X-sidebarPad, yy+rowH)
		m.rects[id] = r
		yy += rowH + gap
		return r
	}
	halfRowRect := func(id gmID) image.Rectangle {
		r := image.Rect(panel.Min.X+sidebarPad, yy, panel.Max.X-sidebarPad, yy+halfH)
		m.rects[id] = r
		yy += halfH + gap
		return r
	}
	stepRowAt := func(step gmStep, r image.Rectangle) {
		pillW := Profile().DensityValues().SidebarValuePillW
		plusX := r.Max.X - sidebarIncBtnW
		pillX := plusX - gap - pillW
		minusX := pillX - gap - sidebarIncBtnW
		incH := sidebarIncBtnH
		if incH > rowH {
			incH = rowH
		}
		btnY := r.Min.Y + (rowH-incH)/2
		m.rects[gmMinus(step)] = image.Rect(minusX, btnY, minusX+sidebarIncBtnW, btnY+incH)
		m.rects[gmValue(step)] = image.Rect(pillX, btnY, pillX+pillW, btnY+incH)
		m.rects[gmPlus(step)] = image.Rect(plusX, btnY, plusX+sidebarIncBtnW, btnY+incH)
	}
	stepRow := func(step gmStep) image.Rectangle {
		r := rowRect(gmRow(step))
		stepRowAt(step, r)
		return r
	}

	halfRowRect(gmRuleLabel)

	ruleR := rowRect(gmRuleOnOff)
	halfW := (ruleR.Dx() - gap) / 2
	m.rects[gmRuleOnOff] = image.Rect(ruleR.Min.X, ruleR.Min.Y, ruleR.Min.X+halfW, ruleR.Max.Y)
	nR := image.Rect(ruleR.Min.X+halfW+gap, ruleR.Min.Y, ruleR.Max.X, ruleR.Max.Y)
	m.rects[gmRow(gmStepEveryN)] = nR
	stepRowAt(gmStepEveryN, nR)

	// Fixed pitch/volume/duration order, matching writeRules.
	stepRow(gmStepPitch)
	stepRow(gmStepVolume)
	stepRow(gmStepDuration)

	// The last row's lone chip spans the full width.
	if len(mergeCandidates) > 0 {
		halfRowRect(gmMergeLabel)
		for i := 0; i < len(mergeCandidates); i += mergeChipCols {
			end := i + mergeChipCols
			if end > len(mergeCandidates) {
				end = len(mergeCandidates)
			}
			row := rowRect(gmMergeRow(i / mergeChipCols))
			chips := mergeCandidates[i:end]
			if len(chips) == 1 {
				m.rects[gmMergeChip(chips[0].ID)] = row
				continue
			}
			chipHalfW := (row.Dx() - gap) / 2
			m.rects[gmMergeChip(chips[0].ID)] = image.Rect(row.Min.X, row.Min.Y, row.Min.X+chipHalfW, row.Max.Y)
			m.rects[gmMergeChip(chips[1].ID)] = image.Rect(row.Min.X+chipHalfW+gap, row.Min.Y, row.Max.X, row.Max.Y)
		}
	}

	rowRect(gmDelete)

	m.ensureButtons(mergeCandidates)
}

// ─── Button wiring ─────────────────────────────────────────────────────────

func (m *GroupMenu) ensureButtons(mergeCandidates []model.NodeGroup) {
	if _, ok := m.group(); !ok {
		return
	}
	ruleOn := m.rulesEnabled()

	mk := func(id gmID, label string, icon string, onClick func()) {
		r, ok := m.rects[id]
		if !ok || r.Empty() {
			delete(m.btns, id)
			return
		}
		if _, exists := m.btns[id]; !exists {
			m.btns[id] = NewButton(label, DropdownStyle, onClick)
		} else {
			m.btns[id].Text = label
			m.btns[id].OnClick = onClick
		}
		b := m.btns[id]
		b.SetRect(r)
		b.ConsumeOnPress = true
		// Styled text, never the bitmap upscale path, which pixelates.
		b.SetTextRole(RoleBody)
		b.Icon = icon
		if icon != "" {
			b.Text = ""
			b.IconColor = colTextPrimary
		}
	}

	mk(gmClose, "", "close", func() { m.Close() })
	if b := m.btns[gmClose]; b != nil {
		b.IconColor = closeIconColor()
	}

	mk(gmSave, i18n.T(i18n.KeyGroupSave), "", func() { m.save() })
	if b := m.btns[gmSave]; b != nil {
		// The gold label marks the primary action while armed; the face stays neutral.
		b.Disabled = !m.dirty()
		b.TextColor = colAccent
	}

	ruleToggleLabel := i18n.T(i18n.KeyGroupRuleOff)
	if ruleOn {
		ruleToggleLabel = i18n.T(i18n.KeyGroupRuleOn)
	}
	mk(gmRuleOnOff, ruleToggleLabel, "", func() { m.setRuleEnabled(!ruleOn) })
	if b := m.btns[gmRuleOnOff]; b != nil {
		b.SetToggled(ruleOn)
	}

	mk(gmMinus(gmStepEveryN), "", "minus", func() { m.stepRuleEveryN(-1) })
	mk(gmPlus(gmStepEveryN), "", "plus", func() { m.stepRuleEveryN(1) })

	mk(gmMinus(gmStepPitch), "", "minus", func() { m.stepRuleDelta(model.GroupParamPitch, -1) })
	mk(gmPlus(gmStepPitch), "", "plus", func() { m.stepRuleDelta(model.GroupParamPitch, 1) })
	mk(gmMinus(gmStepVolume), "", "minus", func() { m.stepRuleDelta(model.GroupParamVolume, -1) })
	mk(gmPlus(gmStepVolume), "", "plus", func() { m.stepRuleDelta(model.GroupParamVolume, 1) })
	mk(gmMinus(gmStepDuration), "", "minus", func() { m.stepRuleDelta(model.GroupParamDuration, -1) })
	mk(gmPlus(gmStepDuration), "", "plus", func() { m.stepRuleDelta(model.GroupParamDuration, 1) })

	candSet := make(map[model.GroupID]bool, len(mergeCandidates))
	for _, cand := range mergeCandidates {
		cand := cand
		candSet[cand.ID] = true
		label := cand.Name
		if label == "" {
			label = i18n.T(i18n.KeyGroupMenuTitle)
		}
		mk(gmMergeChip(cand.ID), label, "", func() { m.mergeInto(cand.ID) })
	}
	// Also delete from m.btns, so a dropped candidate's closure (with a stale gid) never lingers.
	for id := range m.btns {
		if id.el == gmElMergeChip && !candSet[model.GroupID(id.arg)] {
			delete(m.btns, id)
		}
	}

	mk(gmDelete, i18n.T(i18n.KeyGroupDelete), "", func() { m.deleteGroup() })
	if b := m.btns[gmDelete]; b != nil {
		b.TextColor = colError
	}
}

// gripHovered reports whether the desktop cursor is over the drag grip.
func (m *GroupMenu) gripHovered() bool {
	if Profile().IsMobile() {
		return false
	}
	gr := m.rects[gmGrip]
	if gr.Empty() {
		return false
	}
	mx, my := cursorPosition()
	return image.Pt(mx, my).In(gr)
}

// Identity lives in the swatch and hover tint, never a color-filled face.
func (m *GroupMenu) drawMergeChip(dst *ebiten.Image, id gmID, gid model.GroupID) {
	b := m.btns[id]
	if b == nil {
		return
	}
	state := menuItemRest
	if b.hovered || b.pressed {
		state = menuItemHover
	}
	acc := m.game.groupColor(gid)
	drawMenuRow(dst, b, MenuRowSpec{
		Accent: acc,
		State:  state,
		Swatch: acc,
		Label:  b.Text,
	})
}

// ─── Draw ──────────────────────────────────────────────────────────────────

func (m *GroupMenu) Draw(dst *ebiten.Image) {
	if !m.open {
		return
	}
	m.layoutAt(m.anchorX, m.anchorY)
	grp, ok := m.group()
	if !ok {
		return
	}

	panel := m.panel
	drawPanelShadow(dst, panel, 6)
	drawRoundedRect(dst, panel, colPanelBG, RadiusContainer, true)
	// A neutral hairline: the group's identity is on the title swatch and underline.
	drawRoundedRect(dst, panel, colPanelBorder, RadiusContainer, false)

	if gr := m.rects[gmGrip]; !gr.Empty() {
		DrawIcon(dst, IconGrip, gr, popupGripTint(m.drag.Dragging(), m.gripHovered()))
	}

	if r, ok := m.rects[gmTitle]; ok {
		band := image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+sidebarHeaderH)
		drawMenuHeaderBandAccent(dst, band, m.game.groupColor(m.groupID))
		drawMenuTitle(dst, band, m.headerLine(grp), m.game.groupColor(m.groupID))
	}
	m.drawBtn(dst, gmClose)
	m.drawBtn(dst, gmSave)

	if r, ok := m.rects[gmRuleLabel]; ok {
		ty := r.Min.Y + (r.Dy()-StyledTextHeight(RoleCaption))/2
		DrawTextStyled(dst, i18n.T(i18n.KeyGroupRule), r.Min.X, ty, RoleCaption, colTextSecondary)
	}
	m.drawBtn(dst, gmRuleOnOff)
	// Shared by every param's rule; the pill explains itself.
	m.drawStepRow(dst, gmStepEveryN, "", m.everyNPillLabel(m.sharedEveryN()))

	// Three independent per-param delta rows.
	pit, _ := m.ruleFor(model.GroupParamPitch)
	vol, _ := m.ruleFor(model.GroupParamVolume)
	dur, _ := m.ruleFor(model.GroupParamDuration)
	m.drawStepRow(dst, gmStepPitch, i18n.T(i18n.KeyGroupParamPitch), fmt.Sprintf("%+.2f", pit.Delta))
	m.drawStepRow(dst, gmStepVolume, i18n.T(i18n.KeyGroupParamVolume), fmt.Sprintf("%+.2f", vol.Delta))
	m.drawStepRow(dst, gmStepDuration, i18n.T(i18n.KeyGroupParamDuration), fmt.Sprintf("%+.2f", dur.Delta))

	if cands := m.mergeChipCandidates(); len(cands) > 0 {
		if r, ok := m.rects[gmMergeLabel]; ok {
			ty := r.Min.Y + (r.Dy()-StyledTextHeight(RoleCaption))/2
			DrawTextStyled(dst, i18n.T(i18n.KeyGroupAddTo), r.Min.X, ty, RoleCaption, colTextSecondary)
		}
		for _, cand := range cands {
			m.drawMergeChip(dst, gmMergeChip(cand.ID), cand.ID)
		}
	}

	m.drawBtn(dst, gmDelete)
}

// Keeps the text off the pill's rounded border.
const everyNPillLabelFitMargin = 4

// The short form when it fits, else the bare number: the long sentence overflowed onto the "n-"
// button drawn before it.
func (m *GroupMenu) everyNPillLabel(n int) string {
	label := fmt.Sprintf(i18n.T(i18n.KeyGroupEveryNShort), n)
	pillW := 0
	if r, ok := m.rects[gmValue(gmStepEveryN)]; ok {
		pillW = r.Dx()
	}
	if pillW > 0 && StyledTextWidth(label, RoleBody) > pillW-everyNPillLabelFitMargin {
		return fmt.Sprintf("%d", n)
	}
	return label
}

// Truncated to clear the header's buttons, so a long name never overlaps them.
func (m *GroupMenu) headerLine(grp model.NodeGroup) string {
	full := m.titleText() + " · " + fmt.Sprintf(i18n.T(i18n.KeyGroupMembers), len(grp.NodeIDs))
	headerR, ok := m.rects[gmTitle]
	if !ok {
		return full
	}
	rightEdge := headerR.Max.X
	if r, ok := m.rects[gmSave]; ok {
		rightEdge = r.Min.X
	} else if r, ok := m.rects[gmClose]; ok {
		rightEdge = r.Min.X
	}
	// The leading title swatch (drawMenuTitle) takes its share of the band.
	avail := rightEdge - headerR.Min.X - sidebarGap - (menuTitleSwatchSz + SpaceSM)
	if avail <= 0 {
		return full
	}
	return clipTextToWidthMeasure(full, avail,
		func(s string) int { return StyledTextWidth(s, RoleSectionHeader) })
}

func (m *GroupMenu) drawStepRow(dst *ebiten.Image, step gmStep, label, value string) {
	if r, ok := m.rects[gmRow(step)]; ok && label != "" {
		ty := r.Min.Y + (r.Dy()-StyledTextHeight(RoleCaption))/2
		DrawTextStyled(dst, label, r.Min.X, ty, RoleCaption, colTextSecondary)
	}
	m.drawBtn(dst, gmMinus(step))
	if r, ok := m.rects[gmValue(step)]; ok {
		drawRecessedPill(dst, r, value)
	}
	m.drawBtn(dst, gmPlus(step))
}

func (m *GroupMenu) drawBtn(dst *ebiten.Image, id gmID) {
	b := m.btns[id]
	if b == nil {
		return
	}
	b.Draw(dst)
}

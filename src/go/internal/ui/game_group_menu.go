package ui

import (
	"fmt"
	"image"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// groupMenuW is the fixed panel width. Unlike NodeSidebar (left-docked, full
// grid height, scrollable), GroupMenu is a compact anchored popup — no
// scroll, no collapse — so a single constant suffices.
const groupMenuW = 300

// GroupMenu is the anchored panel opened after a marquee selection (or from
// a sidebar group chip). It edits ONE group: the rule editor writes the
// group's Rules through the model API (SetGroupRules/ClearGroupRules). The
// per-node "edit all members" batch steppers (vol/pit/dur) were removed —
// per-node editing lives in the node sidebar; this menu now edits only the
// group's rule set.
type GroupMenu struct {
	game    *Game
	open    bool
	groupID model.GroupID
	anchorX int
	anchorY int
	panel   image.Rectangle
	rects   map[gmID]image.Rectangle
	btns    map[gmID]*Button
	// drag is the top-left grip handle — same contract as NodeSidebar.drag:
	// zero value passes placement through, a drag pins the card to the grid
	// pane, openAt/Close reset it.
	drag PopupDragHandle
	// Rule-section scratch state: the three independent per-param deltas plus
	// the shared EveryN. Authoritative rule state is always read from the
	// live group (grp.Rules — see ruleFor/rulesEnabled/sharedEveryN); this
	// scratch is only the write-target seed for params the group doesn't
	// currently have a rule for (e.g. re-enabling after Off, or composing a
	// brand-new rule from a delta stepper). Reseeded from the live group's
	// Rules every OpenAt/OpenAtProvisional (never per-frame — see openAt).
	ruleDeltaVol float64
	ruleDeltaPit float64
	ruleDeltaDur float64
	ruleEveryN   int
	// savedRules is the deep-copy snapshot of the group's Rules taken at
	// openAt time (both flavors). Compared (order-insensitively, ignoring
	// AnchorIdx — see rulesEqual) against the LIVE group's current Rules to
	// derive dirty(): the pending-edits state the always-present Save button
	// greys out against. Reset to the current Rules on every successful
	// save() (see save()'s doc comment) so the button greys out again right
	// after a commit. Restored INTO the model (silently, no emit/undo) by
	// Close() when the menu closes on a dirty committed group without ever
	// pressing Save — see restoreIfDirty.
	savedRules []model.GroupRule
	// savedEnabled is the snapshot of the group's RulesEnabled status bit
	// (task-enabled-status brief, Part C) taken at the same openAt time as
	// savedRules, and compared against the live group's RulesEnabled by
	// dirty() alongside the Rules comparison — toggling the status is a
	// pending edit exactly like a delta edit, dirtying the always-present
	// Save button and reverting silently on close-without-save
	// (restoreIfDirty). Reset to the just-committed value on every
	// successful save() (mirrors savedRules).
	savedEnabled bool
	// memberRectsScratch is a reused backing slice for the group's
	// per-member screen rects, rebuilt (truncated, not reallocated) every
	// layoutAt via groupMemberScreenRects — feeds placeGroupMenu's per-member
	// overlap test (task-placement-v2).
	memberRectsScratch []image.Rectangle
	// provisional marks a group that was just marquee-created and not yet
	// committed via Save. While true: rule edits made through this menu
	// mutate the model (so rings/rules work live) but do NOT emit
	// emitGroupChanged (no undo step for uncommitted settings — see
	// writeRules). Any Close() (close button, click-outside, node click,
	// sidebar chip, vanish, Import) while still provisional silently deletes
	// the group with no emit/undo step, since the creation itself was never
	// committed. Save() emits the single emitGroupCreated undo step for the
	// whole creation, then clears provisional before closing.
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

// OpenAt opens the menu for group id anchored near (x, y), clamped on-screen.
// Non-provisional: the group is treated as already committed (existing
// callers — sidebar chip reopen, JS export, etc.).
func (m *GroupMenu) OpenAt(id model.GroupID, x, y int) {
	m.openAt(id, x, y, false)
}

// OpenAtProvisional opens the menu for a freshly marquee-created group that
// has not yet been committed. The marquee release path calls this instead of
// OpenAt. See the provisional field's doc comment for the full lifecycle.
func (m *GroupMenu) OpenAtProvisional(id model.GroupID, x, y int) {
	m.openAt(id, x, y, true)
}

// openAt is the shared body for OpenAt/OpenAtProvisional. provisional must be
// set BEFORE layoutAt runs so the initial layout already includes (or
// omits) the "save" control — a caller reading m.rects/m.btns right after
// OpenAt(Provisional) returns must see the correct state without waiting for
// a subsequent Draw()/HandleInput() to re-run layoutAt.
func (m *GroupMenu) openAt(id model.GroupID, x, y int, provisional bool) {
	if _, ok := m.game.graph.Group(id); !ok {
		return
	}
	// A second marquee/open request arriving while a still-open provisional
	// group's menu hasn't been Saved would otherwise overwrite groupID
	// without ever discarding the first group: it leaks into g.graph.Groups
	// uncommitted, menuless, and exportable. Reuse the same silent-discard
	// Close() uses (delete group, ignore ErrGroupNotFound, no emits) so the
	// two paths can't drift.
	if m.open && m.provisional && m.groupID != id {
		m.discardProvisional()
	}
	m.open = true
	m.provisional = provisional
	m.drag.Reset()
	m.groupID = id
	// Reseed the rule-section scratch from the live group's Rules (never
	// left over from whatever group this GroupMenu instance last edited).
	// Missing params default to 0 (Off-shaped); EveryN defaults to 1 when the
	// group has no rules at all.
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
		// Snapshot for the transactional dirty-check (see dirty()'s doc
		// comment). grp.Rules is already a fresh deep copy (model.Graph.Group
		// returns one), so assigning it directly is safe — no aliasing with
		// the live *NodeGroup's slice.
		m.savedRules = grp.Rules
		m.savedEnabled = grp.RulesEnabled
	}
	m.anchorX, m.anchorY = x, y
	m.layoutAt(x, y)
	m.game.dispatcherDirty = true
}

// discardProvisional silently deletes the currently-referenced provisional
// group, if any: DeleteGroup with no emit and no undo step, since the
// creation was never committed. Idempotent and safe to call re-entrantly
// (the model's group-changed hook can call back into Close() while
// DeleteGroup is still unwinding — see onGroupsChanged): the "group still
// exists" check makes a nested call a no-op. Shared by Close() (menu closing
// on an uncommitted group) and openAt() (a second marquee/open arriving
// while a still-open provisional menu would otherwise orphan the first
// group) so the two discard paths can't drift apart.
//
// The group's number becomes immediately reusable once DeleteGroup removes
// it: model.Graph.CreateGroup allocates the lowest free id (lowestFreeGroupID
// — task-save-always Part 2), so the next marquee re-mints the same default
// "Group N" name instead of drifting upward forever on every abandoned
// selection. No separate release call is needed (unlike the old
// nextGroupID/ReleaseGroupID tail-rollback allocator this replaced).
func (m *GroupMenu) discardProvisional() {
	if !m.provisional {
		return
	}
	if _, ok := m.game.graph.Group(m.groupID); ok {
		_ = m.game.graph.DeleteGroup(m.groupID) // silent: ignore ErrGroupNotFound races
	}
	m.provisional = false
}

// restoreIfDirty silently reverts the model to the savedRules/savedEnabled
// snapshot taken at open time, when the group is committed (non-provisional)
// and has pending (unsaved) edits — the "cancel" half of the transactional
// lifecycle (task-save-always Part 1, extended by task-enabled-status Part C
// to also cover the independent RulesEnabled status bit): closing without
// Save must leave zero trace, mirroring discardProvisional's zero-trace
// contract for the provisional case. No emit, no undo step —
// SetGroupRules/ClearGroupRules/SetGroupRulesEnabled still fire the model's
// group-changed hook (index refresh + parity-gen bump via onGroupsChanged),
// which is desired, but that is NOT the same as the ui emit helpers
// (emitGroupChanged), which are never called here. Restores WITH the
// snapshotted AnchorIdx values (savedRules is exactly what model.Graph.Group
// returned at open time), so cancel doesn't perturb ramp state either.
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

// Close closes the menu. Provisional (never-saved) groups are silently
// discarded via discardProvisional, same as before. A committed group with
// pending edits has them silently reverted via restoreIfDirty (cancel
// semantics); a clean committed group just closes.
//
// Guarded at the top by `if !m.open { return }`, and m.open is set to false
// BEFORE discardProvisional/restoreIfDirty run (not after, as a naive port of
// the pre-task-save-always ordering would do): restoreIfDirty's dirty() calls
// group(), which itself calls Close() when the group has vanished and
// m.open is still true — with the old after-the-fact ordering this would
// recurse unboundedly (group() sees m.open still true on every reentrant
// Close() call, so it keeps calling Close() again). Closing m.open first
// makes every reentrant Close() (whether from this model hook chain or the
// pre-existing discardProvisional/DeleteGroup reentrancy — see its doc
// comment) an immediate no-op, which is also a strictly simpler idempotency
// guarantee than the old "let it recurse harmlessly" reasoning.
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

// HandleWheel: the menu carries no scrollable content, so wheel events pass
// through untouched.
func (m *GroupMenu) HandleWheel(_, _, _ int) InputResult { return InputIgnored }

// Capturing: true only while a grip drag is in flight (the menu has no
// scrollable content, so this is its sole capture state).
func (m *GroupMenu) Capturing() bool { return m.drag.Dragging() }

// HandleInput processes mouse/touch input within the menu panel.
func (m *GroupMenu) HandleInput(x, y int, pressed bool) InputResult {
	if !m.open {
		return InputIgnored
	}
	// Refresh rects/buttons against the live model state (rule on/off, param,
	// member count) before hit-testing, mirroring NodeSidebar.layout() being
	// re-run at the top of its HandleInput.
	m.layoutAt(m.anchorX, m.anchorY)

	// Grip drag — highest precedence inside the card. Returning InputCaptured
	// makes the dispatcher route every later frame straight here without
	// re-hit-testing (input_dispatch.go), so the drag survives the cursor
	// leaving the card.
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
		// The dispatcher only calls HandleInput for points inside
		// InputBounds() (== m.panel), so this branch is defensive: it never
		// fires through the normal dispatch path, but any direct caller that
		// hands us an outside point gets the same click-outside-closes
		// contract as the rest of the UI.
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
	// Merge chips (task-merge-chips brief): dynamic ids, one per candidate
	// group — appended before "delete" so the click order mirrors the visual
	// stacking order (rule rows, then merge chips, then delete).
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
	// Opaque to z: any press inside the panel that landed on no control is
	// still consumed so it can't fall through to the grid beneath.
	return InputConsumed
}

// group returns the live group; closes the menu if it vanished (auto-delete
// on last-member removal, or explicit DeleteGroup).
func (m *GroupMenu) group() (model.NodeGroup, bool) {
	grp, ok := m.game.graph.Group(m.groupID)
	if !ok && m.open {
		m.Close()
	}
	return grp, ok
}

// titleText resolves the header title shown in Draw: the group's own name
// when it has one, falling back to the generic i18n title for unnamed
// groups or a vanished group (spec §4 gap — the header used to always show
// the static i18n title regardless of the group's actual Name).
func (m *GroupMenu) titleText() string {
	grp, ok := m.group()
	if !ok || grp.Name == "" {
		return i18n.T(i18n.KeyGroupMenuTitle)
	}
	return grp.Name
}

// ruleFor returns the group's live rule for param (rule, true) when one
// exists, else a rule synthesized from the menu's scratch state (rule,
// false) — used both as the write-time seed and the Draw-time pill value.
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

// scratchDelta reads the menu's per-param scratch delta (the write-target
// seed for a param the live group doesn't currently have a rule for).
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

// rulesEqual reports whether two rule sets are equivalent for dirty-tracking
// purposes: same set of Params, each with matching Delta/EveryN, compared
// order-insensitively and ignoring AnchorIdx — AnchorIdx is runtime ramp
// state (re-stamped on every fresh play-start by
// model.Graph.ReanchorAllGroupRules and, for an in-place edit, preserved by
// writeRules), never a document delta, so it must never make an open menu
// look dirty (task-save-always Part 1 test 5).
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

// dirty reports whether the menu has pending (unsaved) edits: always true
// while provisional (the creation itself is pending), else whether the live
// group's Rules OR its independent RulesEnabled status bit (task-enabled-
// status brief, Part C) have diverged from the snapshot taken at open time.
// This is what the always-present Save button's Disabled state reflects (see
// ensureButtons) and what Close() uses to decide whether a committed group
// needs its rules/status silently restored (see restoreIfDirty).
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

// rulesEnabled reports the live group's independent RulesEnabled status bit
// (task-enabled-status brief, Part C) — the "Enabled"/"Disabled" state the
// ruleonoff button reflects. Deliberately NOT len(grp.Rules)>0: a group can
// carry configured deltas while Disabled (deltas alone never imply enabled —
// see setRuleEnabled/stepRuleDelta).
func (m *GroupMenu) rulesEnabled() bool {
	grp, ok := m.group()
	return ok && grp.RulesEnabled
}

// sharedEveryN returns the EveryN every live rule shares, or the menu's
// scratch EveryN when the group currently has no rules.
func (m *GroupMenu) sharedEveryN() int {
	if grp, ok := m.group(); ok && len(grp.Rules) > 0 {
		return grp.Rules[0].EveryN
	}
	return m.ruleEveryN
}

// writeRules composes up to 3 GroupRules — one per param (fixed order
// pitch/volume/duration, for deterministic export byte-stability) whose
// scratch delta is nonzero — sharing m.ruleEveryN. All-zero deltas write
// nothing, clearing the group's Rules slice instead (an all-zero scratch
// means every delta was reset to 0 — writeRules NEVER touches RulesEnabled;
// see setRuleEnabled for the independent status toggle, task-enabled-status
// brief Part C).
//
// AnchorIdx is PRESERVED per-param when the group already has a live rule for
// that param (an in-place delta/EveryN edit continues the existing ramp —
// task-rule-continuity Part 2): a mid-playback tweak must not reset the
// accumulated step count back to 0. Only a param that is GAINING a rule for
// the first time (the group currently has no rule for that param) gets a
// fresh anchor via ruleAnchorIdxNow, so its own ramp starts from the moment
// it was written. See GroupEffect's doc comment for the step-accumulation
// formula this anchor feeds.
//
// Emission (task-save-always Part 1): rule edits ALWAYS mutate the model
// live (so rings/rules preview correctly, audible immediately — the anchored/
// continuity behavior above is otherwise untouched) but NEVER emit here —
// emitGroupChanged only fires from save() now, exactly once per commit,
// whether the group is provisional (deferred to emitGroupCreated, as before)
// or already committed (previously gated on !m.provisional; now unconditional
// no-emit, matching provisional's existing behavior). A committed group's
// pending edits are undone by Close()'s restoreIfDirty if the menu closes
// without Save.
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

// setRuleEnabled toggles the group's INDEPENDENT RulesEnabled status bit
// (task-enabled-status brief, Part C) via the model's SetGroupRulesEnabled —
// it never touches Rules. Live preview: enabling with nonzero deltas already
// configured is immediately audible (SetGroupRulesEnabled fires the group-
// changed hook, which rebuilds the GroupIndex and, while playing, drops
// in-flight audio so the very next re-predicted trigger reflects the new
// gate); disabling silences the ramp back to identity without discarding the
// configured deltas. Never emits directly (see writeRules' doc comment) —
// only save() emits, and only when the flag (or the rules) actually changed
// relative to the openAt snapshot.
func (m *GroupMenu) setRuleEnabled(on bool) {
	g := m.game
	id := m.groupID
	g.enqueueUI(func() {
		_ = g.graph.SetGroupRulesEnabled(id, on)
	})
}

// stepRuleDelta nudges one param's scratch delta (pitch steps by 1.0
// semitone; volume/duration step by 0.05) and writes the rule. Deliberately
// does NOT touch RulesEnabled (task-enabled-status brief, Part C: delta
// edits must never implicitly enable a group) — the toggle is the only path
// that flips the status bit; see setRuleEnabled.
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

// save commits pending edits (task-save-always Part 1): a provisional group
// emits the single emitGroupCreated undo step for the whole creation
// (membership + any rule edits already applied to the model while
// provisional) and clears provisional; an already-committed group with
// pending edits (dirty() — Rules OR RulesEnabled changed, task-enabled-status
// brief Part C) emits a single emitGroupChanged step. Either way the
// savedRules/savedEnabled snapshot is reset to the group's now-current state
// and the menu STAYS OPEN — the always-present Save button greys out (see
// ensureButtons) once there's nothing left to commit. wasProvisional is
// captured before the enqueued closure runs so dirty()'s provisional branch
// (always-true) reflects the PRE-save state when deciding whether to emit.
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
			// Silent discard: identical to closing without Save — delegate to
			// Close() so discardProvisional() owns the delete. Pre-clearing
			// the flag or deleting inline here would bypass discardProvisional
			// entirely (the memory-pinned invariant: every provisional-discard
			// path must flow through it — the ghost-group bug, via the Delete
			// button).
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

// mergeChipCandidateCap caps the number of "Add to group" merge chips shown
// in the provisional popup (task-merge-chips brief: more groups than this is
// beyond v1 — no scrolling in this popup, just the lowest `mergeChipCandidateCap`
// group ids).
const mergeChipCandidateCap = 4

// mergeChipCols is the number of chips laid out side-by-side per row (see
// layoutAt's merge-chip section).
const mergeChipCols = 2

// mergeChipCandidates returns, in ascending GroupID order and capped at
// mergeChipCandidateCap, every OTHER existing group that does NOT already
// contain every member of the provisional group's current selection (the
// provisional group's own NodeIDs ARE the marquee selection — that's what
// "provisional" means: a real group already created holding those members,
// not yet committed). A group that already contains the whole selection is
// skipped — merging into it would be a no-op (nothing left to add). Returns
// nil unless the menu is currently provisional: chips only ever appear on the
// provisional popup (task-merge-chips brief §Design).
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

// mergeInto implements the "Add to group" merge chip tap (task-merge-chips
// brief §Merge semantics): dissolves the provisional group silently through
// the existing discardProvisional funnel, adds every member of the
// just-dissolved selection to gid (AddNodeToGroup is idempotent for members
// gid already has — see TestMergeChipOverlappingSelection), wraps the whole
// membership change in ONE undo step (beginUndoGroup/endUndoGroup around a
// single emitGroupChanged call after the loop, mirroring the sidebar's
// addToGroup), and reopens gid's menu non-provisional (OpenAt) so the merge
// result — gid's color + strong member highlight — is immediately visible.
//
// Runs inside one enqueued closure (the mk-wired onClick pattern used by
// save/deleteGroup/setRuleEnabled) so the member set is captured BEFORE the
// provisional group is discarded: reading grp.NodeIDs after discardProvisional
// would see nothing (the group is gone). discardProvisional deleting the
// still-open provisional group synchronously triggers the model's
// group-changed hook, which (Game.onGroupsChanged) closes this same menu
// re-entrantly — see discardProvisional's and Close's doc comments for why
// that re-entrant Close is a no-op past the point of deleting the group and
// does not double-discard. The subsequent OpenAt call re-opens fresh,
// non-provisional, on gid.
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
		// Emit only when the merge actually mutated the target — the
		// sidebar's addToGroup convention. A target deleted between chip
		// layout and this closure running (every add ErrGroupNotFound) must
		// not publish a "group updated" event for a nonexistent group.
		if added {
			emitGroupChanged(gid)
		}
		endUndoGroup()
		m.OpenAt(gid, anchorX, anchorY)
	})
}

// ─── Layout ────────────────────────────────────────────────────────────────

// groupMenuFullRows is the number of full-height control rows in the
// compressed layout: the combined rule on/off+everyN row, the three
// per-param rule delta rows (rdpit/rdvol/rddur — task-rule-anchor Part B),
// and delete. The per-node "edit all members" batch steppers (vol/pit/dur)
// were removed (task-rule-continuity Part 1) — per-node editing lives in
// the node sidebar; this menu edits only the group's rule set.
const groupMenuFullRows = 5

// groupMenuHalfRows is the number of half-height section-label rows: the
// "Rule" label, drawn as small inline text above its control block rather
// than consuming a full button row (see layoutAt's row list) — folding this
// (and the old standalone "members" row, merged into the header by
// headerLine) out of the fixed row count is what makes the panel fit the
// grid pane without needing to shrink rows at ordinary window sizes.
const groupMenuHalfRows = 1

// computeGroupMenuRowSizing derives the row height/gap pair that makes the
// GroupMenu's full row stack fit within maxH (the grid pane height). Starts
// at the sidebar's ideal sizing (sidebarBtnH/sidebarGap) and shrinks gap
// first, then row height, down to floors (24px row / 3px gap — still
// legible/tappable). If a window is so small even the floors overflow, keeps
// shrinking evenly below the floors as a last resort: the panel must NEVER
// overflow the grid pane, like the node sidebar.
//
// extraHalfRows/extraFullRows let a caller fold in the "Add to group" merge
// chip section's rows (task-merge-chips brief: 1 half-row label + ceil(n/2)
// full chip rows, present only while provisional with >=1 candidate) so the
// fit-shrinking loop accounts for the taller panel instead of computing a
// sizing that only the base rows would fit in. Pass 0, 0 for the base layout
// (no merge section).
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

// layoutAt recomputes rects for a fixed-width panel anchored near (x, y),
// clamped inside the grid pane on BOTH axes (position and size — see
// computeGroupMenuRowSizing), then wires the buttons against the fresh rects
// and current model state. Reuses the NodeSidebar's pad/header constants
// (sidebarPad, sidebarHeaderH, ...) so the two menus share the same keycap
// rhythm — GroupMenu is deliberately NOT a scrollable clone, just a compact
// adaptive stack of rows that always fits the pane, like the sidebar does.
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

	// Merge-chip candidates (task-merge-chips brief): computed BEFORE row
	// sizing so the "Add to group" section's extra rows (1 half-row label +
	// ceil(n/mergeChipCols) chip rows) are folded into the fit-shrinking pass
	// below, exactly like the base rows always have been. Empty (nil) unless
	// the menu is provisional with >=1 addable candidate.
	mergeCandidates := m.mergeChipCandidates()
	extraHalfRows, extraChipRows := 0, 0
	if n := len(mergeCandidates); n > 0 {
		extraHalfRows = 1
		extraChipRows = (n + mergeChipCols - 1) / mergeChipCols
	}

	rowH, gap := computeGroupMenuRowSizing(gridH, extraHalfRows, extraChipRows)
	halfH := rowH / 2

	// Rows, top to bottom: header (title + live member count merged into one
	// line — see headerLine), a half-row "Rule" label, the combined rule
	// on/off+everyN row, the three per-param delta rows, an optional "Add to
	// group" section (half-row label + chip rows, provisional only), delete.
	h := sidebarPad*2 + sidebarHeaderH + gap +
		(groupMenuHalfRows+extraHalfRows)*(halfH+gap) + (groupMenuFullRows+extraChipRows)*(rowH+gap)

	// Single label-proximal placement pass (task-placement-v2; see
	// placeGroupMenu's doc comment for the full rationale) — replaces the old
	// pane-clamp + avoidRect(member bbox) + avoidReservedRect(gear) chain,
	// which could (a) fall back to an already-overlapping panel when the
	// member bbox was too wide/tall for any of avoidRect's four fixed
	// obstacle-relative candidates to fit, and (b) have its trailing gear
	// re-application push an avoidRect-cleared panel straight back onto a
	// member. placeGroupMenu tests candidates adjacent to the ANCHOR (not the
	// obstacle) and re-checks per-member overlap AFTER the gear adjustment,
	// closing both gaps. Members can move/scroll (camera pan) while the menu
	// is open; layoutAt re-runs every frame, so the panel keeps tracking
	// them, matching the old behavior.
	reserved := g.gridHelpReservedRect()
	members := []image.Rectangle(nil)
	if grp, ok := g.graph.Group(m.groupID); ok {
		m.memberRectsScratch = g.groupMemberScreenRects(grp.NodeIDs, m.memberRectsScratch)
		members = m.memberRectsScratch
	}
	// While PINNED, skip auto-placement entirely rather than computing it and
	// overriding: placeGroupMenu's member-overlap pass would otherwise re-run
	// every frame against a result it can't affect, and a member drifting under
	// the card must not shove it off the user's drop point.
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
	// Title starts clear of the grip glyph; Save/close keep their right-aligned
	// header slots, so only the left edge moves.
	titleR := headerR
	if gr := m.rects[gmGrip]; !gr.Empty() {
		if x := gr.Max.X + sidebarGap; x > titleR.Min.X {
			titleR.Min.X = x
		}
	}
	m.rects[gmTitle] = titleR
	closeSize := sidebarBtnH
	// Close × is desktop-only (shared desktopCloseVisible gate). On mobile the
	// group menu dismisses via tap-away, so no rect is published; mk(gmClose)
	// below self-prunes its button, and Save reclaims the freed header space.
	if desktopCloseVisible() {
		m.rects[gmClose] = image.Rect(headerR.Max.X-closeSize, headerR.Min.Y, headerR.Max.X, headerR.Min.Y+closeSize)
	} else {
		delete(m.rects, gmClose)
	}

	// Save button: header row, immediately left of close. ALWAYS present
	// (task-save-always Part 1) — provisional or committed, dirty or clean;
	// ensureButtons sets Button.Disabled = !dirty() rather than pruning the
	// control, since there's now always something to represent (either the
	// pending creation, or a greyed-out "nothing to commit" state).
	label := i18n.T(i18n.KeyGroupSave)
	textW := StyledTextWidth(label, RoleBody)
	saveW := textW + 4*SpaceXS
	if minW := closeSize * 2; saveW < minW {
		saveW = minW
	}
	// Save's right edge sits just left of the close × on desktop; on mobile (no
	// close ×) it reclaims the full header width up to the right padding.
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
	// halfRowRect is a compact section-label row (see groupMenuHalfRows'
	// doc comment) — half the height of a control row, no buttons.
	halfRowRect := func(id gmID) image.Rectangle {
		r := image.Rect(panel.Min.X+sidebarPad, yy, panel.Max.X-sidebarPad, yy+halfH)
		m.rects[id] = r
		yy += halfH + gap
		return r
	}
	// stepRowAt wires a −/value/+ trio into an arbitrary rect r (either a
	// full-width row from rowRect, or a half-width split of one — see the
	// combined delta+everyN row below).
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

	// ruleonoff (left half) + the shared Every-N stepper (right half) share
	// one row — the param cycle button that used to occupy this row's right
	// half is gone (task-rule-anchor Part B); the freed half now hosts
	// Every-N instead of it living combined with delta on its own row.
	ruleR := rowRect(gmRuleOnOff)
	halfW := (ruleR.Dx() - gap) / 2
	m.rects[gmRuleOnOff] = image.Rect(ruleR.Min.X, ruleR.Min.Y, ruleR.Min.X+halfW, ruleR.Max.Y)
	nR := image.Rect(ruleR.Min.X+halfW+gap, ruleR.Min.Y, ruleR.Max.X, ruleR.Max.Y)
	m.rects[gmRow(gmStepEveryN)] = nR
	stepRowAt(gmStepEveryN, nR)

	// Three independent per-param delta rows, fixed order pitch/volume/
	// duration (matches writeRules' export-order) — replacing the single
	// param-cycle delta row.
	stepRow(gmStepPitch)
	stepRow(gmStepVolume)
	stepRow(gmStepDuration)

	// "Add to group" merge-chip section (task-merge-chips brief): one label
	// half-row + ceil(n/mergeChipCols) chip rows, mergeChipCols chips per row
	// (the last row may hold fewer, in which case its lone chip spans the
	// full row width). Rects are keyed "mergechip:<gid>" — see
	// mergeChipCandidates for the candidate list and mergeInto for the tap
	// handler wired in ensureButtons.
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
		// Smooth styled label (embedded Inter) — never the legacy bitmap
		// upscale path, which pixelated every GroupMenu button caption.
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
		// Always present (task-save-always Part 1); greyed out via
		// Button.Disabled when there are no pending edits to commit — see
		// dirty()'s doc comment. Button.Draw/HandleInputResult already know
		// how to render/ignore a Disabled button (uigrid.go). The face stays
		// a neutral keycap; the gold accent LABEL marks it as the panel's
		// primary action when armed (disabled draws colTextDisabled).
		b.Disabled = !m.dirty()
		b.TextColor = colAccent
	}

	ruleToggleLabel := i18n.T(i18n.KeyGroupRuleOff)
	if ruleOn {
		ruleToggleLabel = i18n.T(i18n.KeyGroupRuleOn)
	}
	mk(gmRuleOnOff, ruleToggleLabel, "", func() { m.setRuleEnabled(!ruleOn) })
	if b := m.btns[gmRuleOnOff]; b != nil {
		// An enabled rule reads as the standard latched amber keycap — the
		// same "this is on" state every toggle in the app uses.
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

	// "Add to group" merge chips (task-merge-chips brief): one keycap per
	// candidate group, id "mergechip:<gid>", label = the group's own Name (or
	// the generic i18n title for unnamed groups, mirroring the sidebar's
	// grpchip fallback). Tinted with the candidate's OWN groupColor in Draw
	// (tintMergeChipAccent), same as the sidebar's grpchip styling.
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
	// Prune stale merge-chip buttons: a candidate that scrolled out of the
	// cap, got merged away, or stopped qualifying (e.g. now contains every
	// selected member) no longer has a rect (mk's own empty-rect check
	// deletes it) — but also delete straight out of m.btns so a dropped
	// candidate's button/closure (capturing a stale gid) never lingers.
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

// drawMergeChip renders one "Add to group" candidate as a flat menu row:
// the candidate group's identity lives in the leading swatch + hover tint
// (never a color-filled face), matching the sidebar's membership chips and
// the shared list-menu row language.
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
	// Standard neutral panel hairline — the group's identity moves to the
	// title swatch + header underline (the shared identity-menu treatment),
	// off the panel ring, so the chrome stays calm.
	drawRoundedRect(dst, panel, colPanelBorder, RadiusContainer, false)

	if gr := m.rects[gmGrip]; !gr.Empty() {
		DrawIcon(dst, IconGrip, gr, popupGripTint(m.drag.Dragging(), m.gripHovered()))
	}

	// Title: unified menu title (leading group-color swatch + section-header
	// role — DESIGN.md "Menu anatomy") over the group's own name + live
	// member count, truncated to fit beside the Save/close buttons. The
	// accent hairline under the header band uses the group color, mirroring
	// the node sidebar's instrument underline.
	if r, ok := m.rects[gmTitle]; ok {
		band := image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+sidebarHeaderH)
		drawMenuHeaderBandAccent(dst, band, m.game.groupColor(m.groupID))
		drawMenuTitle(dst, band, m.headerLine(grp), m.game.groupColor(m.groupID))
	}
	m.drawBtn(dst, gmClose)
	m.drawBtn(dst, gmSave)

	// Rule section — "Rule" is a half-row inline label above the combined
	// on/off+everyN row. This is the menu's only editable section
	// (task-rule-continuity Part 1 removed the "edit all members" batch
	// steppers that used to sit above it).
	if r, ok := m.rects[gmRuleLabel]; ok {
		ty := r.Min.Y + (r.Dy()-StyledTextHeight(RoleCaption))/2
		DrawTextStyled(dst, i18n.T(i18n.KeyGroupRule), r.Min.X, ty, RoleCaption, colTextSecondary)
	}
	m.drawBtn(dst, gmRuleOnOff)
	// Every-N is shared across every param's rule (task-rule-anchor Part B —
	// the param cycle button that used to share this row is gone); label
	// stays empty, the pill is self-explanatory ("every N rounds").
	m.drawStepRow(dst, gmStepEveryN, "", m.everyNPillLabel(m.sharedEveryN()))

	// Three independent per-param delta rows.
	pit, _ := m.ruleFor(model.GroupParamPitch)
	vol, _ := m.ruleFor(model.GroupParamVolume)
	dur, _ := m.ruleFor(model.GroupParamDuration)
	m.drawStepRow(dst, gmStepPitch, i18n.T(i18n.KeyGroupParamPitch), fmt.Sprintf("%+.2f", pit.Delta))
	m.drawStepRow(dst, gmStepVolume, i18n.T(i18n.KeyGroupParamVolume), fmt.Sprintf("%+.2f", vol.Delta))
	m.drawStepRow(dst, gmStepDuration, i18n.T(i18n.KeyGroupParamDuration), fmt.Sprintf("%+.2f", dur.Delta))

	// "Add to group" merge chips (task-merge-chips brief) — provisional only,
	// only when at least one addable candidate exists (see
	// mergeChipCandidates). Reuses KeyGroupAddTo, the same label the
	// sidebar's "Add to group" row already carries.
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

// everyNPillLabelFitMargin is the slack (px) subtracted from the pill's
// width before comparing against the candidate label's measured width — a
// small buffer so the text never touches the pill's rounded border.
const everyNPillLabelFitMargin = 4

// everyNPillLabel resolves the Every-N pill's display text: the short form
// ("Every %d" via KeyGroupEveryNShort — see task-groupsave Every-N row fix)
// when it fits the pill rect, else the bare number. The long-form
// KeyGroupEveryN sentence ("Every %d rounds") is too wide for the half-row
// pill at every density and used to overflow past the pill's left edge,
// painting over the "n-" button drawn just before it in drawStepRow (the
// button's rect was always present and clickable — only the paint order made
// it look missing).
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

// headerLine composes the header's display text: the group's title (see
// titleText) plus the live member count (formerly its own "members" row),
// truncated to fit the header rect minus whatever close/save buttons occupy
// on its right edge so a long group name never overlaps or overflows past
// them.
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

// drawStepRow draws a batch/rule stepper row's left-side label (when non-
// empty) and the −/value/+ trio.
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

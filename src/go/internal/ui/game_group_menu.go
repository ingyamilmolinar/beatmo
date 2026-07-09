package ui

import (
	"fmt"
	"image"
	"image/color"

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
	rects   map[string]image.Rectangle
	btns    map[string]*Button
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
		rects:      map[string]image.Rectangle{},
		btns:       map[string]*Button{},
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
	m.groupID = id
	// Reseed the rule-section scratch from the live group's Rules (never
	// left over from whatever group this GroupMenu instance last edited).
	// Missing params default to 0 (Off-shaped); EveryN defaults to 1 when the
	// group has no rules at all.
	m.ruleDeltaVol, m.ruleDeltaPit, m.ruleDeltaDur = 0, 0, 0
	m.ruleEveryN = 1
	m.savedRules = nil
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

// restoreIfDirty silently reverts the model to the savedRules snapshot taken
// at open time, when the group is committed (non-provisional) and has
// pending (unsaved) rule edits — the "cancel" half of the transactional
// lifecycle (task-save-always Part 1): closing without Save must leave zero
// trace, mirroring discardProvisional's zero-trace contract for the
// provisional case. No emit, no undo step — SetGroupRules/ClearGroupRules
// still fire the model's group-changed hook (index refresh + parity-gen bump
// via onGroupsChanged), which is desired, but that is NOT the same as the ui
// emit helpers (emitGroupChanged), which are never called here. Restores
// WITH the snapshotted AnchorIdx values (savedRules is exactly what
// model.Graph.Group returned at open time), so cancel doesn't perturb ramp
// state either.
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
		return
	}
	_ = m.game.graph.SetGroupRules(id, m.savedRules)
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
	m.btns = map[string]*Button{}
	m.rects = map[string]image.Rectangle{}
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

// Capturing: no drag-capture state (unlike the sidebar's scroll thumb).
func (m *GroupMenu) Capturing() bool { return false }

// HandleInput processes mouse/touch input within the menu panel.
func (m *GroupMenu) HandleInput(x, y int, pressed bool) InputResult {
	if !m.open {
		return InputIgnored
	}
	// Refresh rects/buttons against the live model state (rule on/off, param,
	// member count) before hit-testing, mirroring NodeSidebar.layout() being
	// re-run at the top of its HandleInput.
	m.layoutAt(m.anchorX, m.anchorY)

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

	order := []string{
		"save", "close",
		"ruleonoff", "n-", "n+", "rdpit-", "rdpit+", "rdvol-", "rdvol+", "rddur-", "rddur+", "delete",
	}
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
// group's Rules have diverged from the savedRules snapshot taken at open
// time. This is what the always-present Save button's Disabled state
// reflects (see ensureButtons) and what Close() uses to decide whether a
// committed group needs its rules silently restored (see restoreIfDirty).
func (m *GroupMenu) dirty() bool {
	if m.provisional {
		return true
	}
	grp, ok := m.group()
	if !ok {
		return false
	}
	return !rulesEqual(grp.Rules, m.savedRules)
}

// rulesEnabled reports whether the live group currently has any rules (the
// "On" state the ruleonoff button reflects).
func (m *GroupMenu) rulesEnabled() bool {
	grp, ok := m.group()
	return ok && len(grp.Rules) > 0
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
// nothing, clearing the group's rules instead (shows Off — see the type doc
// comment's "On = write current deltas" contract).
//
// AnchorIdx is PRESERVED per-param when the group already has a live rule for
// that param (an in-place delta/EveryN edit continues the existing ramp —
// task-rule-continuity Part 2): a mid-playback tweak must not reset the
// accumulated step count back to 0. Only a param that is GAINING a rule for
// the first time (the group currently has none for it — including the
// Off->On case, where the group has no rules at all) gets a fresh anchor via
// ruleAnchorIdxNow, so its own ramp starts from the moment it was written.
// See GroupEffect's doc comment for the step-accumulation formula this anchor
// feeds.
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

// setRuleEnabled: Off clears the group's rules outright (regardless of
// scratch deltas). On writes whatever the current scratch deltas compose to
// — an all-zero scratch (never edited) stays Off, per the type doc comment.
// Never emits (see writeRules' doc comment) — only save() emits.
func (m *GroupMenu) setRuleEnabled(on bool) {
	if !on {
		g := m.game
		id := m.groupID
		g.enqueueUI(func() {
			_ = g.graph.ClearGroupRules(id)
		})
		return
	}
	m.writeRules()
}

// stepRuleDelta nudges one param's scratch delta (pitch steps by 1.0
// semitone; volume/duration step by 0.05) and writes — implicitly enabling
// if the group was Off (matches the pre-redesign single-rule behavior).
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
// pending edits (dirty()) emits a single emitGroupChanged step. Either way
// the savedRules snapshot is reset to the group's now-current Rules and the
// menu STAYS OPEN — the always-present Save button greys out (see
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
		} else if !rulesEqual(grp.Rules, m.savedRules) {
			emitGroupChanged(id)
		}
		m.savedRules = grp.Rules
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
func computeGroupMenuRowSizing(maxH int) (rowH, gap int) {
	rowH, gap = sidebarBtnH, sidebarGap
	contentH := func(rh, gp int) int {
		halfH := rh / 2
		return sidebarPad*2 + sidebarHeaderH + gp +
			groupMenuHalfRows*(halfH+gp) + groupMenuFullRows*(rh+gp)
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
	m.rects = map[string]image.Rectangle{}

	gridW := g.split.GridW(g.winW)
	gridH := g.split.GridH(g.winH)

	w := groupMenuW
	if w > gridW {
		w = gridW
	}
	if w < 0 {
		w = 0
	}

	rowH, gap := computeGroupMenuRowSizing(gridH)
	halfH := rowH / 2

	// Rows, top to bottom: header (title + live member count merged into one
	// line — see headerLine), a half-row "Rule" label, the combined rule
	// on/off+everyN row, the three per-param delta rows, delete.
	h := sidebarPad*2 + sidebarHeaderH + gap +
		groupMenuHalfRows*(halfH+gap) + groupMenuFullRows*(rowH+gap)

	ox, oy := x, y
	if ox+w > gridW {
		ox = gridW - w
	}
	if ox < 0 {
		ox = 0
	}
	if oy+h > gridH {
		oy = gridH - h
	}
	if oy < 0 {
		oy = 0
	}

	panel := image.Rect(ox, oy, ox+w, oy+h)
	// Keep the panel clear of the settings gear's protected top-right zone —
	// the child rects below derive from panel, so they follow.
	panel = avoidReservedRect(panel, g.gridHelpReservedRect(), gridW, gridH)
	m.panel = panel
	m.rects["panel"] = panel

	headerR := image.Rect(panel.Min.X+sidebarPad, panel.Min.Y+sidebarPad, panel.Max.X-sidebarPad, panel.Min.Y+sidebarPad+sidebarHeaderH)
	m.rects["title"] = headerR
	closeSize := sidebarBtnH
	m.rects["close"] = image.Rect(headerR.Max.X-closeSize, headerR.Min.Y, headerR.Max.X, headerR.Min.Y+closeSize)

	// Save button: header row, immediately left of close. ALWAYS present
	// (task-save-always Part 1) — provisional or committed, dirty or clean;
	// ensureButtons sets Button.Disabled = !dirty() rather than pruning the
	// control, since there's now always something to represent (either the
	// pending creation, or a greyed-out "nothing to commit" state).
	label := i18n.T(i18n.KeyGroupSave)
	textW := int(float64(TextWidth(label)) * sidebarLabelScale())
	saveW := textW + 4*SpaceXS
	if minW := closeSize * 2; saveW < minW {
		saveW = minW
	}
	saveX2 := m.rects["close"].Min.X - sidebarGap
	saveX1 := saveX2 - saveW
	if saveX1 < headerR.Min.X {
		saveX1 = headerR.Min.X
	}
	m.rects["save"] = image.Rect(saveX1, headerR.Min.Y, saveX2, headerR.Min.Y+closeSize)

	yy := headerR.Max.Y + gap
	rowRect := func(id string) image.Rectangle {
		r := image.Rect(panel.Min.X+sidebarPad, yy, panel.Max.X-sidebarPad, yy+rowH)
		m.rects[id] = r
		yy += rowH + gap
		return r
	}
	// halfRowRect is a compact section-label row (see groupMenuHalfRows'
	// doc comment) — half the height of a control row, no buttons.
	halfRowRect := func(id string) image.Rectangle {
		r := image.Rect(panel.Min.X+sidebarPad, yy, panel.Max.X-sidebarPad, yy+halfH)
		m.rects[id] = r
		yy += halfH + gap
		return r
	}
	// stepRowAt wires a −/value/+ trio into an arbitrary rect r (either a
	// full-width row from rowRect, or a half-width split of one — see the
	// combined delta+everyN row below).
	stepRowAt := func(base string, r image.Rectangle) {
		pillW := Profile().DensityValues().SidebarValuePillW
		plusX := r.Max.X - sidebarIncBtnW
		pillX := plusX - gap - pillW
		minusX := pillX - gap - sidebarIncBtnW
		incH := sidebarIncBtnH
		if incH > rowH {
			incH = rowH
		}
		btnY := r.Min.Y + (rowH-incH)/2
		m.rects[base+"-"] = image.Rect(minusX, btnY, minusX+sidebarIncBtnW, btnY+incH)
		m.rects[base+"val"] = image.Rect(pillX, btnY, pillX+pillW, btnY+incH)
		m.rects[base+"+"] = image.Rect(plusX, btnY, plusX+sidebarIncBtnW, btnY+incH)
	}
	stepRow := func(base string) image.Rectangle {
		r := rowRect(base)
		stepRowAt(base, r)
		return r
	}

	halfRowRect("rulelabel")

	// ruleonoff (left half) + the shared Every-N stepper (right half) share
	// one row — the param cycle button that used to occupy this row's right
	// half is gone (task-rule-anchor Part B); the freed half now hosts
	// Every-N instead of it living combined with delta on its own row.
	ruleR := rowRect("ruleonoff")
	halfW := (ruleR.Dx() - gap) / 2
	m.rects["ruleonoff"] = image.Rect(ruleR.Min.X, ruleR.Min.Y, ruleR.Min.X+halfW, ruleR.Max.Y)
	nR := image.Rect(ruleR.Min.X+halfW+gap, ruleR.Min.Y, ruleR.Max.X, ruleR.Max.Y)
	m.rects["n"] = nR
	stepRowAt("n", nR)

	// Three independent per-param delta rows, fixed order pitch/volume/
	// duration (matches writeRules' export-order) — replacing the single
	// param-cycle delta row.
	stepRow("rdpit")
	stepRow("rdvol")
	stepRow("rddur")

	rowRect("delete")

	m.ensureButtons()
}

// ─── Button wiring ─────────────────────────────────────────────────────────

func (m *GroupMenu) ensureButtons() {
	if _, ok := m.group(); !ok {
		return
	}
	ruleOn := m.rulesEnabled()

	mk := func(id, label string, icon string, onClick func()) {
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
		b.TextScale = sidebarLabelScale()
		b.Icon = icon
		if icon != "" {
			b.Text = ""
			b.IconColor = colTextPrimary
		}
	}

	mk("close", "", "close", func() { m.Close() })
	if b := m.btns["close"]; b != nil {
		b.IconColor = closeIconColor()
	}

	mk("save", i18n.T(i18n.KeyGroupSave), "", func() { m.save() })
	if b := m.btns["save"]; b != nil {
		// Always present (task-save-always Part 1); greyed out via
		// Button.Disabled when there are no pending edits to commit — see
		// dirty()'s doc comment. Button.Draw/HandleInputResult already know
		// how to render/ignore a Disabled button (uigrid.go).
		b.Disabled = !m.dirty()
	}

	ruleToggleLabel := i18n.T(i18n.KeyGroupRuleOff)
	if ruleOn {
		ruleToggleLabel = i18n.T(i18n.KeyGroupRuleOn)
	}
	mk("ruleonoff", ruleToggleLabel, "", func() { m.setRuleEnabled(!ruleOn) })

	mk("n-", "", "minus", func() { m.stepRuleEveryN(-1) })
	mk("n+", "", "plus", func() { m.stepRuleEveryN(1) })

	mk("rdpit-", "", "minus", func() { m.stepRuleDelta(model.GroupParamPitch, -1) })
	mk("rdpit+", "", "plus", func() { m.stepRuleDelta(model.GroupParamPitch, 1) })
	mk("rdvol-", "", "minus", func() { m.stepRuleDelta(model.GroupParamVolume, -1) })
	mk("rdvol+", "", "plus", func() { m.stepRuleDelta(model.GroupParamVolume, 1) })
	mk("rddur-", "", "minus", func() { m.stepRuleDelta(model.GroupParamDuration, -1) })
	mk("rddur+", "", "plus", func() { m.stepRuleDelta(model.GroupParamDuration, 1) })

	mk("delete", i18n.T(i18n.KeyGroupDelete), "", func() { m.deleteGroup() })
	if b := m.btns["delete"]; b != nil {
		b.TextColor = colError
	}
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
	drawRoundedRect(dst, panel, colPanelBG, RadiusLG, true)
	drawRoundedRect(dst, panel, WithAlpha(genColorSidebarSectionBg, genAlphaSidebarSection), RadiusLG, false)
	// The group's own color (groupColor) layered on as a border accent — the
	// same "node pop-up picks up node colors" language the sidebar uses
	// (tintBtnAccent), applied here to the panel chrome instead of a node.
	drawRoundedRect(dst, panel, WithAlpha(m.game.groupColor(m.groupID), AlphaStrong), RadiusLG, false)

	// Title: the group's own name (or the i18n fallback — see titleText, spec
	// §4) merged with the live member count into one header line (the old
	// standalone "members" row was folded in here — task-groupsave
	// layout-fit fix). Truncated to fit beside the close/save buttons.
	if r, ok := m.rects["title"]; ok {
		title := m.headerLine(grp)
		ty := r.Min.Y + (sidebarHeaderH-StyledTextHeight(RoleSectionHeader))/2
		DrawTextStyled(dst, title, r.Min.X, ty, RoleSectionHeader, colTextPrimary)
	}
	m.drawBtn(dst, "close")
	m.tintSaveBtnAccent()
	m.drawBtn(dst, "save")

	// Rule section — "Rule" is a half-row inline label above the combined
	// on/off+everyN row. This is the menu's only editable section
	// (task-rule-continuity Part 1 removed the "edit all members" batch
	// steppers that used to sit above it).
	if r, ok := m.rects["rulelabel"]; ok {
		ty := r.Min.Y + (r.Dy()-StyledTextHeight(RoleCaption))/2
		DrawTextStyled(dst, i18n.T(i18n.KeyGroupRule), r.Min.X, ty, RoleCaption, colTextSecondary)
	}
	m.drawBtn(dst, "ruleonoff")
	// Every-N is shared across every param's rule (task-rule-anchor Part B —
	// the param cycle button that used to share this row is gone); label
	// stays empty, the pill is self-explanatory ("every N rounds").
	m.drawStepRow(dst, "n", "", m.everyNPillLabel(m.sharedEveryN()))

	// Three independent per-param delta rows.
	pit, _ := m.ruleFor(model.GroupParamPitch)
	vol, _ := m.ruleFor(model.GroupParamVolume)
	dur, _ := m.ruleFor(model.GroupParamDuration)
	m.drawStepRow(dst, "rdpit", i18n.T(i18n.KeyGroupParamPitch), fmt.Sprintf("%+.2f", pit.Delta))
	m.drawStepRow(dst, "rdvol", i18n.T(i18n.KeyGroupParamVolume), fmt.Sprintf("%+.2f", vol.Delta))
	m.drawStepRow(dst, "rddur", i18n.T(i18n.KeyGroupParamDuration), fmt.Sprintf("%+.2f", dur.Delta))

	m.drawBtn(dst, "delete")
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
	if r, ok := m.rects["nval"]; ok {
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
	headerR, ok := m.rects["title"]
	if !ok {
		return full
	}
	rightEdge := headerR.Max.X
	if r, ok := m.rects["save"]; ok {
		rightEdge = r.Min.X
	} else if r, ok := m.rects["close"]; ok {
		rightEdge = r.Min.X
	}
	avail := rightEdge - headerR.Min.X - sidebarGap
	if avail <= 0 {
		return full
	}
	return truncateName(full, avail, sidebarLabelScale())
}

// drawStepRow draws a batch/rule stepper row's left-side label (when non-
// empty) and the −/value/+ trio.
func (m *GroupMenu) drawStepRow(dst *ebiten.Image, base, label, value string) {
	if r, ok := m.rects[base]; ok && label != "" {
		ty := r.Min.Y + (r.Dy()-StyledTextHeight(RoleCaption))/2
		DrawTextStyled(dst, label, r.Min.X, ty, RoleCaption, colTextSecondary)
	}
	m.drawBtn(dst, base+"-")
	if r, ok := m.rects[base+"val"]; ok {
		drawRecessedPill(dst, r, value)
	}
	m.drawBtn(dst, base+"+")
}

func (m *GroupMenu) drawBtn(dst *ebiten.Image, id string) {
	b := m.btns[id]
	if b == nil {
		return
	}
	b.Draw(dst)
}

// tintSaveBtnAccent gives the Save button's keycap FACE the group's own
// color (groupColor) as a tint, mirroring NodeSidebar.tintBtnAccent (same
// blend-from-DropdownStyle-base, same sidebarBtnAccentTint strength) so the
// GroupMenu's one commit action reads as owned by this group the same way
// the sidebar's controls read as owned by their node's instrument.
func (m *GroupMenu) tintSaveBtnAccent() {
	b := m.btns["save"]
	if b == nil {
		return
	}
	if bs, ok := b.Style.(ButtonStyle); ok {
		base := color.RGBAModel.Convert(DropdownStyle.Fill).(color.RGBA)
		acc := m.game.groupColor(m.groupID)
		bs.Fill = blendColor(base, acc, sidebarBtnAccentTint)
		b.Style = bs
	}
}

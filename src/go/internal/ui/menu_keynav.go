package ui

import "github.com/hajimehoshi/ebiten/v2"

// One keyboard navigation for every list menu, read in each menu's per-frame portal hook, so
// DrumViewTree dispatch is untouched. A menu using it must claim the keyboard while open, or the arrows
// also pan the grid.

// Addressed by the menu's OWN row indices, non-choices included; MenuItemSelectable lets a menu
// interleave headers and pinned chrome, so "next item" is not "next index".
type MenuKeyNavHost interface {
	// MenuItemCount is the total number of rows, selectable or not.
	MenuItemCount() int
	// Headers, separators and pinned search rows return false.
	MenuItemSelectable(i int) bool
	// MenuActivate performs row i's action (what a click would do).
	MenuActivate(i int)
	// nil for menus that never scroll. *ScrollBehavior, the common denominator: the context menu has
	// no MenuScroll.
	MenuScrollBehavior() *ScrollBehavior
}

// A grid host moves Up/Down by a row and Left/Right by a cell, matching what the user sees.
type MenuKeyNavGrid interface {
	// MenuGridCols is the number of columns; <= 1 means "treat me as a list".
	MenuGridCols() int
}

// For hosts whose scroll unit is not the item index (a grid scrolls rows while the cursor counts cells).
type MenuKeyNavRevealer interface {
	MenuRevealItem(i int)
}

// Shift+Enter fires a row's second action (the ★ toggle); without it Shift+Enter falls back to
// MenuActivate, never a dead key.
type MenuKeyNavSecondary interface {
	MenuSecondary(i int)
}

// Menus open with nothing highlighted: a highlight on appearance implies a default the user never chose.
const MenuKeyNavNoSelection = -1

// ONE highlight per menu, shared with the mouse (hover calls SetSelected), so Enter always acts on
// the row the user sees highlighted. Not concurrent-safe; one per menu.
type MenuKeyNav struct {
	sel     int
	keyHold map[ebiten.Key]keyHoldState
	// Lets SetSelected reject non-selectable rows; bound by BindHost or the first Update.
	host MenuKeyNavHost
}

// NewMenuKeyNav returns a nav with nothing selected.
func NewMenuKeyNav() *MenuKeyNav {
	return &MenuKeyNav{sel: MenuKeyNavNoSelection, keyHold: map[ebiten.Key]keyHoldState{}}
}

// held alone cannot express "a full interval passed since the last repeat", which keeps the
// cadence monotonic.
type keyHoldState struct {
	held      int
	lastFired int
}

// Call explicitly when hover may run before a fresh menu's first Update.
func (n *MenuKeyNav) BindHost(h MenuKeyNavHost) { n.host = h }

// Selected returns the highlighted row index, or MenuKeyNavNoSelection.
func (n *MenuKeyNav) Selected() int { return n.sel }

// Non-selectable rows are ignored, so hovering a header does not blank the highlight.
func (n *MenuKeyNav) SetSelected(i int) {
	if i == MenuKeyNavNoSelection {
		n.sel = i
		return
	}
	if n.host != nil {
		if i < 0 || i >= n.host.MenuItemCount() || !n.host.MenuItemSelectable(i) {
			return
		}
	}
	n.sel = i
}

// Call when the menu closes or its list changes shape, so a stale index cannot activate the wrong
// row. Also forgets held keys, or a reopen mid-hold repeats with no initial delay.
func (n *MenuKeyNav) Reset() {
	n.sel = MenuKeyNavNoSelection
	for k := range n.keyHold {
		delete(n.keyHold, k)
	}
}

// Once per frame while open. Rising-edge, so a held arrow cannot race through 60 rows a second.
func (n *MenuKeyNav) Update(h MenuKeyNavHost) {
	if h == nil {
		return
	}
	if n.keyHold == nil {
		n.keyHold = map[ebiten.Key]keyHoldState{}
	}
	n.host = h
	// Clamp a cursor left dangling by a list that shrank under it.
	if n.sel >= h.MenuItemCount() || (n.sel >= 0 && !h.MenuItemSelectable(n.sel)) {
		n.sel = MenuKeyNavNoSelection
	}

	// Both arrow spellings: real Ebiten aliases them, the test stub declares four distinct keys.
	// onEdgeAny folds aliases into ONE slot so the real build cannot double-fire. Grid hosts move a row
	// vertically and a cell horizontally.
	if cols := menuKeyNavCols(h); cols > 1 {
		n.onHeldAny(func() { n.stepBy(h, +cols, +1) }, ebiten.KeyArrowDown, ebiten.KeyDown)
		n.onHeldAny(func() { n.stepBy(h, -cols, -1) }, ebiten.KeyArrowUp, ebiten.KeyUp)
		n.onHeldAny(func() { n.stepBy(h, +1, +1) }, ebiten.KeyArrowRight, ebiten.KeyRight)
		n.onHeldAny(func() { n.stepBy(h, -1, -1) }, ebiten.KeyArrowLeft, ebiten.KeyLeft)
	} else {
		n.onHeldAny(func() { n.step(h, +1) }, ebiten.KeyArrowDown, ebiten.KeyDown)
		n.onHeldAny(func() { n.step(h, -1) }, ebiten.KeyArrowUp, ebiten.KeyUp)
	}
	// Home/End are absolute jumps — repeating them lands on the same row.
	n.onEdgeAny(func() { n.jump(h, +1) }, ebiten.KeyHome)
	n.onEdgeAny(func() { n.jump(h, -1) }, ebiten.KeyEnd)
	n.onHeldAny(func() { n.page(h, +1) }, ebiten.KeyPageDown)
	n.onHeldAny(func() { n.page(h, -1) }, ebiten.KeyPageUp)
	n.onEdgeAny(func() {
		if n.sel < 0 || n.sel >= h.MenuItemCount() || !h.MenuItemSelectable(n.sel) {
			return
		}
		// Shift as its two sides, never ebiten.KeyShift: the test stub has no KeyShift, so that binding
		// would be dead under every test.
		if menuKeyNavShiftHeld() {
			if s, ok := h.(MenuKeyNavSecondary); ok {
				s.MenuSecondary(n.sel)
				return
			}
		}
		h.MenuActivate(n.sel)
	}, ebiten.KeyEnter)
}

// Aliases share keys[0]'s slot, or real Ebiten (where they are one key) would repeat at double speed.
// No repeat for Enter (it would preview, confirm and close in one hold) or Home/End (absolute jumps).
func (n *MenuKeyNav) onEdgeOrRepeat(fn func(), repeat bool, keys ...ebiten.Key) {
	if len(keys) == 0 {
		return
	}
	now := false
	for _, k := range keys {
		if isKeyPressed(k) {
			now = true
			break
		}
	}
	slot := keys[0]
	if !now {
		// Released: drop the slot so the next press waits the full delay again.
		delete(n.keyHold, slot)
		return
	}
	st := n.keyHold[slot]
	st.held++
	fire := st.held == 1 // the press edge
	if !fire && repeat && menuRepeat.dueSince(st.held, st.lastFired) {
		fire = true
	}
	if fire {
		st.lastFired = st.held
	}
	// Record BEFORE fn: fn can close the menu and reset the nav, and a later write would resurrect it.
	n.keyHold[slot] = st
	if fire {
		fn()
	}
}

// onEdgeAny is onEdgeOrRepeat for keys that fire once per press.
func (n *MenuKeyNav) onEdgeAny(fn func(), keys ...ebiten.Key) {
	n.onEdgeOrRepeat(fn, false, keys...)
}

// onHeldAny is onEdgeOrRepeat for keys that auto-repeat while held.
func (n *MenuKeyNav) onHeldAny(fn func(), keys ...ebiten.Key) {
	n.onEdgeOrRepeat(fn, true, keys...)
}

// From no selection, Down enters at the first row and Up at the last. Clamps rather than wraps,
// which would loop a held arrow silently.
func menuKeyNavShiftHeld() bool {
	return isKeyPressed(ebiten.KeyShiftLeft) || isKeyPressed(ebiten.KeyShiftRight)
}

// menuKeyNavCols reports a host's grid width, or 0 when it is a plain list.
func menuKeyNavCols(h MenuKeyNavHost) int {
	if g, ok := h.(MenuKeyNavGrid); ok {
		return g.MenuGridCols()
	}
	return 0
}

// For grid hosts, where a vertical move is a whole row.
func (n *MenuKeyNav) stepBy(h MenuKeyNavHost, delta, scanDir int) {
	if h.MenuItemCount() == 0 {
		return
	}
	if n.sel == MenuKeyNavNoSelection {
		if delta > 0 {
			n.selectFrom(h, 0, +1)
		} else {
			n.selectFrom(h, h.MenuItemCount()-1, -1)
		}
		return
	}
	n.selectFrom(h, n.sel+delta, scanDir)
}

func (n *MenuKeyNav) step(h MenuKeyNavHost, dir int) {
	if h.MenuItemCount() == 0 {
		return
	}
	if n.sel == MenuKeyNavNoSelection {
		if dir > 0 {
			n.selectFrom(h, 0, +1)
		} else {
			n.selectFrom(h, h.MenuItemCount()-1, -1)
		}
		return
	}
	n.selectFrom(h, n.sel+dir, dir)
}

// jump sends the cursor to the first (+1) or last (-1) selectable row.
func (n *MenuKeyNav) jump(h MenuKeyNavHost, dir int) {
	if h.MenuItemCount() == 0 {
		return
	}
	if dir > 0 {
		n.selectFrom(h, 0, +1)
	} else {
		n.selectFrom(h, h.MenuItemCount()-1, -1)
	}
}

// page moves by one visible page, then settles on the nearest selectable row.
func (n *MenuKeyNav) page(h MenuKeyNavHost, dir int) {
	span := 1
	if sb := h.MenuScrollBehavior(); sb != nil && sb.VS.Visible > 0 {
		span = sb.VS.Visible
	}
	start := n.sel
	if start == MenuKeyNavNoSelection {
		// No cursor: page the list itself, or the first PageDown on a scrolled menu does nothing.
		if sb := h.MenuScrollBehavior(); sb != nil && sb.VS.Visible > 0 {
			sb.VS.First += dir * span
			sb.VS.Clamp()
			start = sb.VS.First
			if dir < 0 {
				start = sb.VS.First
			}
		} else if dir > 0 {
			start = 0
		} else {
			start = h.MenuItemCount() - 1
		}
	} else {
		start += dir * span
	}
	n.selectFrom(h, start, dir)
}

// Clamps at the end; with no position, takes the nearest selectable row the other way so a jump
// past the end still lands.
func (n *MenuKeyNav) selectFrom(h MenuKeyNavHost, i, dir int) {
	count := h.MenuItemCount()
	if dir == 0 {
		dir = 1
	}
	if i < 0 {
		i = 0
	} else if i >= count {
		i = count - 1
	}
	for j := i; j >= 0 && j < count; j += dir {
		if h.MenuItemSelectable(j) {
			n.sel = j
			n.reveal(h)
			return
		}
	}
	for j := i; j >= 0 && j < count; j -= dir {
		if h.MenuItemSelectable(j) {
			n.sel = j
			n.reveal(h)
			return
		}
	}
}

// Programmatic placement must call this (SetSelected does not reveal: it is the hover path), or the
// cursor sits off-screen with no key to bring it back.
func (n *MenuKeyNav) Reveal(h MenuKeyNavHost) {
	if h == nil {
		return
	}
	n.host = h
	n.reveal(h)
}

func (n *MenuKeyNav) reveal(h MenuKeyNavHost) {
	if n.sel < 0 {
		return
	}
	// A host whose scroll unit is not the item index places its own cursor.
	if r, ok := h.(MenuKeyNavRevealer); ok {
		r.MenuRevealItem(n.sel)
		return
	}
	sb := h.MenuScrollBehavior()
	if sb == nil {
		return
	}
	if sb.VS.Visible <= 0 {
		return
	}
	switch {
	case n.sel < sb.VS.First:
		sb.VS.First = n.sel
	case n.sel >= sb.VS.First+sb.VS.Visible:
		sb.VS.First = n.sel - sb.VS.Visible + 1
	default:
		return
	}
	sb.VS.Clamp()
}

// Lets a table test skip menus too short to repeat.
func (n *MenuKeyNav) MenuItemCountForTest() int {
	if n == nil || n.host == nil {
		return 0
	}
	return n.host.MenuItemCount()
}

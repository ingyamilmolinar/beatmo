package ui

import "github.com/hajimehoshi/ebiten/v2"

// menu_keynav.go — the shared keyboard navigation for every list menu.
//
// Extracted from InstrumentMenuComponent, which was the only menu with arrow
// support (kbMoveDown/kbMoveUp/kbConfirm/kbPageUp/kbPageDown/kbHome/kbEnd +
// scrollToSelection). Every other menu was mouse-only. Rather than copy that
// code six times — the parallel-implementations pattern that produced the
// platform wheel bug — all six menus now drive this one component.
//
// Keys are read in each menu's existing per-frame portal update hook, so this
// needs NO changes to DrumViewTree's dispatch, commonKeys, or HandleKey. The
// one cross-cutting requirement is that a menu using this claims the keyboard
// while open (see keyboard_focus.go), otherwise arrows also pan the grid.

// MenuKeyNavHost is what a menu exposes to gain keyboard navigation.
//
// The list is addressed by the menu's OWN row indices, including rows that are
// not choices. MenuItemSelectable is what lets a menu interleave group headers
// and pinned chrome with its real rows: the Templates menu's item list is
// [title][search][Back][group header][template]... so "the next item" is
// emphatically not "the next index".
type MenuKeyNavHost interface {
	// MenuItemCount is the total number of rows, selectable or not.
	MenuItemCount() int
	// MenuItemSelectable reports whether row i can hold the cursor.
	// Headers, separators and pinned search rows return false.
	MenuItemSelectable(i int) bool
	// MenuActivate performs row i's action (what a click would do).
	MenuActivate(i int)
	// MenuScrollBehavior is the menu's scroll state, used to reveal the cursor
	// when it moves off the visible page. Nil for menus that never scroll.
	//
	// This is *ScrollBehavior rather than *MenuScroll because it is the common
	// denominator: MenuScroll-based menus hand over their ScrollBehavior(),
	// while the context menu owns a bare ScrollBehavior with no MenuScroll.
	MenuScrollBehavior() *ScrollBehavior
}

// MenuKeyNavGrid is an OPTIONAL host interface. A host that lays its items out
// in a grid reports its column count, and the cursor then moves the way the
// grid looks: Up/Down by a whole row, Left/Right by one cell. Hosts that do not
// implement it keep the plain list behaviour unchanged.
type MenuKeyNavGrid interface {
	// MenuGridCols is the number of columns; <= 1 means "treat me as a list".
	MenuGridCols() int
}

// MenuKeyNavRevealer is an OPTIONAL host interface for hosts whose scroll unit
// is not the item index.
//
// reveal's default maths compares the cursor index against VS.First/Visible,
// which is only right when one scroll step == one item. A grid scrolls by ROWS
// while its cursor counts CELLS, so it places its own cursor in view instead.
type MenuKeyNavRevealer interface {
	MenuRevealItem(i int)
}

// MenuKeyNavSecondary is an OPTIONAL host interface for a menu whose rows carry
// a SECOND action alongside "pick this one" — the instrument menu's ★ favourite
// toggle. Shift+Enter fires it on the cursor row.
//
// A host that does not implement it is unaffected: Shift+Enter falls back to
// MenuActivate, so the chord never becomes a dead key in the menus that have
// nothing to bind it to.
type MenuKeyNavSecondary interface {
	MenuSecondary(i int)
}

// MenuKeyNavNoSelection is the cursor value meaning "nothing is highlighted".
// A menu opens in this state: highlighting a row the moment a menu appears
// would imply a default action the user never chose, and Enter would then fire
// an arbitrary row.
const MenuKeyNavNoSelection = -1

// MenuKeyNav owns one menu's highlighted row.
//
// There is exactly ONE highlight per menu, shared with the mouse: hovering a
// row calls SetSelected, so the pointer takes the cursor over rather than
// lighting a second row. Enter therefore always acts on what the user can see
// is highlighted, whichever input moved it there.
//
// Not safe for concurrent use; each menu owns its own.
type MenuKeyNav struct {
	sel     int
	keyHold map[ebiten.Key]keyHoldState
	// host lets SetSelected reject non-selectable rows without every hover site
	// having to re-derive selectability. Bound by BindHost or the first Update.
	host MenuKeyNavHost
}

// NewMenuKeyNav returns a nav with nothing selected.
func NewMenuKeyNav() *MenuKeyNav {
	return &MenuKeyNav{sel: MenuKeyNavNoSelection, keyHold: map[ebiten.Key]keyHoldState{}}
}

// keyHoldState is one edge slot's hold bookkeeping: how many consecutive frames
// it has been down, and the held-count on which it last fired. Both are needed —
// held alone cannot express "a full interval has passed since the last repeat",
// which is what keeps the cadence monotonic (see repeatCurve.dueSince).
type keyHoldState struct {
	held      int
	lastFired int
}

// BindHost associates the menu so SetSelected can consult MenuItemSelectable.
// Update binds automatically; call this explicitly when hover may run before
// the first Update of a newly opened menu.
func (n *MenuKeyNav) BindHost(h MenuKeyNavHost) { n.host = h }

// Selected returns the highlighted row index, or MenuKeyNavNoSelection.
func (n *MenuKeyNav) Selected() int { return n.sel }

// SetSelected moves the cursor, typically because the pointer hovered row i.
// Non-selectable rows (headers, chrome) are ignored so hovering a group header
// does not blank the highlight.
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

// Reset clears the highlight — call when the menu closes or its list changes
// shape (e.g. a new search query), so a stale index cannot activate the wrong
// row after the list underneath it moved.
//
// It also forgets every held key. A menu that closes and reopens while an arrow
// is still physically down would otherwise resume the slot mid-hold: no press
// edge, and repeats firing from the first frame with no initial delay.
func (n *MenuKeyNav) Reset() {
	n.sel = MenuKeyNavNoSelection
	for k := range n.keyHold {
		delete(n.keyHold, k)
	}
}

// Update reads this frame's keys and moves/activates accordingly.
// Call once per frame while the menu is open.
//
// Movement is rising-edge: a held arrow moves one row per press, not one per
// frame, which would race through the list at 60 rows/second.
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

	// Both arrow spellings are accepted. Real Ebiten ALIASES them
	// (KeyDown == KeyArrowDown, keys.go), but the -tags test ebitestub declares
	// KeyUp/KeyDown/KeyArrowUp/KeyArrowDown as four DISTINCT constants. Binding
	// only one spelling therefore works in production yet silently does nothing
	// under the stub for callers using the other — which is exactly how this
	// first shipped broken. onEdgeAny folds the aliases into ONE edge slot so
	// the real build (where they are the same key) cannot double-fire.
	// A grid host moves by a whole row vertically and one cell horizontally, so
	// the cursor tracks what the user sees. Both arrow spellings are bound for
	// the horizontal keys too, for the same ebitestub-alias reason as above.
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
		// Shift+Enter runs the row's secondary action when it has one.
		//
		// Shift is read as the two SIDES, never as ebiten.KeyShift. Real Ebiten
		// does have KeyShift and resolves it to "left OR right"
		// (internal/ui/input.go), so binding it would work in production — but
		// the -tags test ebitestub declares ONLY KeyShiftLeft/KeyShiftRight and
		// no KeyShift at all, so that binding would be dead under every test.
		// Checking both sides is the one spelling that works on both.
		if menuKeyNavShiftHeld() {
			if s, ok := h.(MenuKeyNavSecondary); ok {
				s.MenuSecondary(n.sel)
				return
			}
		}
		h.MenuActivate(n.sel)
	}, ebiten.KeyEnter)
}

// onEdgeOrRepeat runs fn on the frame ANY of keys transitions from up to down,
// and — when repeat is set — again on each tick of the menuRepeat curve while
// the key stays down.
//
// keys[0] is the shared slot, so aliases of the same physical key are debounced
// AND counted together rather than each holding its own state. That matters
// twice over now: under real Ebiten KeyDown and KeyArrowDown are the same key,
// so separate slots would advance the hold counter twice per frame and repeat
// at double speed.
//
// repeat is false for Enter, Home and End. A repeating Enter would preview,
// confirm and close a menu in a single held gesture; Home and End are absolute
// jumps with nowhere to repeat to.
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
	// Record BEFORE running fn: fn can close the menu and reset this nav, and
	// writing afterwards would resurrect the slot it just cleared.
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

// step moves the cursor one selectable row in dir (+1 down, -1 up).
//
// From "nothing selected", Down enters at the first row and Up at the last —
// the direction the user asked to travel decides which end they start from.
// Movement clamps rather than wraps: wrapping makes a held arrow loop the list
// silently, and gives no feedback that an end was reached.
// menuKeyNavShiftHeld reports whether either Shift key is down this frame.
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

// stepBy moves the cursor by delta, then scans in scanDir for the nearest
// selectable cell. Used by grid hosts, where a vertical move is a whole row.
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
		// No cursor yet: page the LIST itself and land on the new page's first
		// row. Entering at row 0 instead would make the first PageDown a no-op
		// on an already-scrolled menu.
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

// selectFrom scans from i in direction dir for the first selectable row and
// takes it. If the scan runs off that end, the cursor stays where it was
// (clamping); if it had no position, it takes the nearest selectable row in
// the OPPOSITE direction so a page/jump past the end still lands somewhere.
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
	// Nothing that way (e.g. trailing headers): try back the other way so a
	// page/End that overshoots still settles on a real row.
	for j := i; j >= 0 && j < count; j -= dir {
		if h.MenuItemSelectable(j) {
			n.sel = j
			n.reveal(h)
			return
		}
	}
}

// Reveal scrolls the menu so the CURRENT cursor is inside the visible page.
//
// Arrow movement reveals as it goes, but SetSelected deliberately does not: it
// is the hover entry point, where the row is visible by definition. A caller
// that places the cursor programmatically — a screenshot scene, a test, an
// "open on the current item" — must call this, or the cursor sits off-screen
// drawing nothing with no keystroke available to bring it back.
func (n *MenuKeyNav) Reveal(h MenuKeyNavHost) {
	if h == nil {
		return
	}
	n.host = h
	n.reveal(h)
}

// reveal scrolls the menu so the cursor is inside the visible page. Without it
// the highlight walks off-screen and the user is steering something invisible.
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

// MenuItemCountForTest reports the bound host's row count, or 0 when unbound.
// Lets a table-driven test skip menus too short to observe a repeat.
func (n *MenuKeyNav) MenuItemCountForTest() int {
	if n == nil || n.host == nil {
		return 0
	}
	return n.host.MenuItemCount()
}

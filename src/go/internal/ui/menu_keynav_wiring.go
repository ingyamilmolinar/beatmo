package ui

// menu_keynav_wiring.go — MenuKeyNavHost adapters for the two list menus that
// DrumView hosts directly (rather than as first-class portal components).
//
// DrumView owns several menus at once, so it cannot implement MenuKeyNavHost
// itself — one type, one interface. Each menu gets a tiny adapter instead,
// keeping the wiring in one file rather than spread across DrumView.

// --- Overflow menu (File page + Templates page) ---------------------------

// overflowNavHost adapts the overflow/template menu to MenuKeyNavHost.
//
// This is the menu that motivated MenuItemSelectable: its item list is
// [Templates title][search box][Back][group header][template][template]...,
// so most indices are not choices at all.
type overflowNavHost struct{ dv *DrumView }

func (h overflowNavHost) MenuItemCount() int { return len(h.dv.overflowItems()) }

func (h overflowNavHost) MenuItemSelectable(i int) bool {
	items := h.dv.overflowItems()
	if i < 0 || i >= len(items) {
		return false
	}
	// Headers are captions and the search row is a text field; neither is a
	// choice, so the cursor steps over them.
	return !items[i].header && !items[i].search && items[i].onClick != nil
}

func (h overflowNavHost) MenuActivate(i int) {
	items := h.dv.overflowItems()
	if i < 0 || i >= len(items) || items[i].onClick == nil {
		return
	}
	items[i].onClick()
}

func (h overflowNavHost) MenuScrollBehavior() *ScrollBehavior {
	if h.dv.overflowMenuScroll == nil {
		return nil
	}
	return h.dv.overflowMenuScroll.ScrollBehavior()
}

// overflowKeyNav lazily creates the overflow menu's keyboard cursor.
func (dv *DrumView) overflowKeyNav() *MenuKeyNav {
	if dv.overflowNav == nil {
		dv.overflowNav = NewMenuKeyNav()
	}
	return dv.overflowNav
}

// updateOverflowKeyNav runs one frame of keyboard navigation for the overflow
// menu. Called from the overflow portal's updateFn.
//
// The cursor is indexed into the CURRENT item list, which changes shape as the
// user types in the search box (rows are filtered) or switches page. Resetting
// on a shape change stops a stale index from activating whatever row happens to
// have slid under it.
func (dv *DrumView) updateOverflowKeyNav() {
	host := overflowNavHost{dv: dv}
	nav := dv.overflowKeyNav()
	nav.BindHost(host)
	sig := dv.overflowNavSignature()
	switch {
	case dv.overflowNavSig == "":
		// First observation of this menu. A real signature always contains the
		// page number, so "" is never one — treating it as a change would reset
		// a cursor placed before the first frame (a screenshot scene, or a
		// caller that opens the menu and preselects a row in the same tick).
		dv.overflowNavSig = sig
	case sig != dv.overflowNavSig:
		// The list changed shape (page switch or new search query), so the
		// retained index now points at a different row than the user saw.
		dv.overflowNavSig = sig
		nav.Reset()
	}
	nav.Update(host)
}

// overflowNavSignature identifies the shape of the current item list.
func (dv *DrumView) overflowNavSignature() string {
	return itoaMenuNav(dv.overflowPage) + "|" + dv.templateSearchQuery
}

// OverflowKeyNavForTest exposes the overflow menu's keyboard cursor.
func (dv *DrumView) OverflowKeyNavForTest() *MenuKeyNav { return dv.overflowKeyNav() }

// --- Row context menu -----------------------------------------------------

// contextNavHost adapts the per-row context menu to MenuKeyNavHost.
//
// Indexed over the BUTTON list rather than the item list: the context menu's
// dividers and group headers own no button, so the button list is already the
// list of choices, and its indices line up with what drawContextMenuItem
// renders — which is what lets the highlight land on the row the user sees.
type contextNavHost struct{ dv *DrumView }

func (h contextNavHost) MenuItemCount() int { return len(h.dv.contextMenuBtns) }

func (h contextNavHost) MenuItemSelectable(i int) bool {
	if i < 0 || i >= len(h.dv.contextMenuBtns) {
		return false
	}
	b := h.dv.contextMenuBtns[i]
	// A disabled row (e.g. Delete on the last remaining row) owns no action.
	return b != nil && b.OnClick != nil && !b.Disabled
}

func (h contextNavHost) MenuActivate(i int) {
	if !h.MenuItemSelectable(i) {
		return
	}
	h.dv.contextMenuBtns[i].OnClick()
}

func (h contextNavHost) MenuScrollBehavior() *ScrollBehavior { return h.dv.contextMenuScroll }

// contextKeyNav lazily creates the context menu's keyboard cursor.
func (dv *DrumView) contextKeyNav() *MenuKeyNav {
	if dv.contextNav == nil {
		dv.contextNav = NewMenuKeyNav()
	}
	return dv.contextNav
}

// updateContextKeyNav runs one frame of keyboard navigation for the row
// context menu. Called from the context portal's updateFn.
func (dv *DrumView) updateContextKeyNav() {
	dv.contextKeyNav().Update(contextNavHost{dv: dv})
}

// ContextKeyNavForTest exposes the context menu's keyboard cursor.
func (dv *DrumView) ContextKeyNavForTest() *MenuKeyNav { return dv.contextKeyNav() }

// itoaMenuNav is a tiny int->string used for the nav signature.
func itoaMenuNav(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	if neg {
		return "-" + string(b)
	}
	return string(b)
}

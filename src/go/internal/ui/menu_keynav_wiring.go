package ui

// DrumView hosts several menus, so each gets a small adapter rather than DrumView implementing
// the interface.

// --- Overflow menu (File page + Templates page) ---------------------------

// Most of its indices are not choices: title, search, Back, group headers, then templates.
type overflowNavHost struct{ dv *DrumView }

func (h overflowNavHost) MenuItemCount() int { return len(h.dv.overflowItems()) }

func (h overflowNavHost) MenuItemSelectable(i int) bool {
	items := h.dv.overflowItems()
	if i < 0 || i >= len(items) {
		return false
	}
	// Headers and the search row are not choices.
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

// The list changes shape as the user types or switches page; reset on a shape change so a stale
// index cannot activate whatever slid under it.
func (dv *DrumView) updateOverflowKeyNav() {
	host := overflowNavHost{dv: dv}
	nav := dv.overflowKeyNav()
	nav.BindHost(host)
	sig := dv.overflowNavSignature()
	switch {
	case dv.overflowNavSig == "":
		// "" is never a real signature, so it must not count as a change: that would reset a cursor placed
		// before the first frame.
		dv.overflowNavSig = sig
	case sig != dv.overflowNavSig:
		dv.overflowNavSig = sig
		nav.Reset()
	}
	nav.Update(host)
}

// overflowNavSignature identifies the shape of the current item list.
func (dv *DrumView) overflowNavSignature() string {
	return itoaMenuNav(dv.overflowPage) + "|" + dv.templateSearchQuery
}

func (dv *DrumView) resetOverflowKeyNav() {
	dv.overflowNavSig = ""
	dv.overflowKeyNav().Reset()
}

// OverflowKeyNavForTest exposes the overflow menu's keyboard cursor.
func (dv *DrumView) OverflowKeyNavForTest() *MenuKeyNav { return dv.overflowKeyNav() }

// --- Row context menu -----------------------------------------------------

// Over the BUTTON list: dividers and headers have no button, so its indices line up with the
// rendered rows.
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

package ui

// Menu width classes (DESIGN.md "Menu sizing & layout"). A floating menu's
// desktop width is chosen by ROLE, never by its content; on mobile every menu
// is a full-width bottom sheet. Values are layout-tied profile tokens
// (profileOverrides.menuW* in DESIGN.md → design_profile.gen.go).

// menuWAction clamps a content-fitted width for an action menu (short verb
// lists: the row context menu, the action menu, subdivision) to its class.
func menuWAction(contentW int) int {
	p := Profile()
	if contentW < p.MenuWActionMin {
		return p.MenuWActionMin
	}
	if contentW > p.MenuWActionMax {
		return p.MenuWActionMax
	}
	return contentW
}

// menuWList is the width of a searchable list menu (instrument picker,
// template page).
func menuWList() int { return Profile().MenuWList }

// menuWEditor is the width of a card hosting controls: the navigation menu's
// go-to wheel, Settings, Theme, the FX panel.
func menuWEditor() int { return Profile().MenuWEditor }

// menuWWide is the width of a data panel (Logs) in a window windowW wide:
// three quarters of the window, at most menuWWideMax.
func menuWWide(windowW int) int {
	w := windowW * 3 / 4
	if max := Profile().MenuWWideMax; w > max {
		return max
	}
	return w
}

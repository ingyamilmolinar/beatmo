package ui

// Desktop width is chosen by ROLE, never content; mobile menus are full-width sheets.

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

func menuWList() int { return Profile().MenuWList }

func menuWEditor() int { return Profile().MenuWEditor }

// Three quarters of the window, at most menuWWideMax.
func menuWWide(windowW int) int {
	w := windowW * 3 / 4
	if max := Profile().MenuWWideMax; w > max {
		return max
	}
	return w
}

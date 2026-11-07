package ui

// "Swipe scrolls, tap opens": drives a LIVE scroll during the gesture and reports the verdict at the end. Not
// DrumView.tapGate (a debounce) nor DeferredTap (which fires a stored position). The zero value is idle.
type dragScrollArbiter struct {
	grid     *ControlGrid
	startY   int
	scrolled bool
	active   bool
}

// A nil or non-scrolling grid still tracks the tap.
func (a *dragScrollArbiter) begin(y int, g *ControlGrid) {
	a.grid = g
	a.startY = y
	a.scrolled = false
	a.active = true
	if g != nil {
		g.BeginContentDrag(y) // natural: content follows the finger
	}
}

func (a *dragScrollArbiter) drag(y int) (moved bool) {
	if !a.active {
		return false
	}
	if abs(y-a.startY) >= tapMaxMovePx {
		a.scrolled = true
	}
	if a.grid != nil {
		return a.grid.DragTo(y)
	}
	return false
}

func (a *dragScrollArbiter) end() (wasTap bool) {
	wasTap = a.active && !a.scrolled
	if a.grid != nil {
		a.grid.EndDrag()
	}
	a.active = false
	a.grid = nil
	return wasTap
}

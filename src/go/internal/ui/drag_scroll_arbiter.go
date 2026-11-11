package ui

// dragScrollArbiter decides, over a single press gesture, whether the pointer
// tapped the control it started on or swept to scroll the ControlGrid that hosts
// that control. On press it begins a natural content drag on the grid; each drag
// frame advances the grid and, once travel crosses the tap slop, latches the
// gesture as a scroll. On release it reports whether the gesture stayed a tap
// (caller opens the control) or became a scroll (caller does nothing). Bound to
// one *ControlGrid for the gesture's lifetime; the zero value is idle.
//
// This is the "swipe scrolls, tap opens" gate shared by the Synth and Sampler
// control adapters. It is deliberately NOT DrumView.tapGate (the mobile tap
// DEBOUNCE keyed by location+frame) nor DeferredTap (which fires a stored
// position on release); this one drives a LIVE scroll during the gesture and
// only reports the tap/scroll verdict at the end.
type dragScrollArbiter struct {
	grid     *ControlGrid
	startY   int
	scrolled bool
	active   bool
}

// begin starts a gesture at pointer y on grid g. A nil or non-scrolling grid
// still tracks the tap so release opens the control.
func (a *dragScrollArbiter) begin(y int, g *ControlGrid) {
	a.grid = g
	a.startY = y
	a.scrolled = false
	a.active = true
	if g != nil {
		g.BeginContentDrag(y) // natural: content follows the finger
	}
}

// drag advances the grid to pointer y and latches a scroll once travel exceeds
// the tap slop. Returns true when the grid's scroll position changed.
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

// end finishes the gesture and reports whether it stayed a tap (never crossed
// the slop). The caller opens the control only on a tap.
func (a *dragScrollArbiter) end() (wasTap bool) {
	wasTap = a.active && !a.scrolled
	if a.grid != nil {
		a.grid.EndDrag()
	}
	a.active = false
	a.grid = nil
	return wasTap
}

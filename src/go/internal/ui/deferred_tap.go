package ui

// DeferredTap encapsulates the two-phase tap pattern used on mobile: position
// is stored on touch begin, then fired on touch end if no scroll was committed.
// This prevents accidental button presses during scroll gestures.
//
// Five instances replace 15 individual fields on DrumView (contextMenu,
// overflow, instMenu, eqCh, subdiv).
type DeferredTap struct {
	active bool
	x, y   int
}

// Begin records a tap start at (x, y). Returns false (and does nothing) if
// suppressClicksUntilRelease is set, ensuring uniform guard behavior across
// all menus.
func (dt *DeferredTap) Begin(x, y int) bool {
	if suppressClicksUntilRelease {
		return false
	}
	dt.active = true
	dt.x, dt.y = x, y
	return true
}

// End fires fireFn with the stored position if a tap was in progress, then
// clears the state. Returns true if a tap was pending.
func (dt *DeferredTap) End(fireFn func(x, y int)) bool {
	if !dt.active {
		return false
	}
	dt.active = false
	if fireFn != nil {
		fireFn(dt.x, dt.y)
	}
	return true
}

// Cancel clears any pending tap without firing.
func (dt *DeferredTap) Cancel() { dt.active = false }

// Active reports whether a tap is pending.
func (dt *DeferredTap) Active() bool { return dt.active }

// Pos returns the stored tap position.
func (dt *DeferredTap) Pos() (int, int) { return dt.x, dt.y }

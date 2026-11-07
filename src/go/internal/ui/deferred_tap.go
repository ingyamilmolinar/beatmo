package ui

// Stored on touch begin, fired on touch end only if no scroll was committed, so a scroll never presses a button.
type DeferredTap struct {
	active bool
	x, y   int
}

// Refuses while suppressClicksUntilRelease is set, the same guard for every menu.
func (dt *DeferredTap) Begin(x, y int) bool {
	if suppressClicksUntilRelease {
		return false
	}
	dt.active = true
	dt.x, dt.y = x, y
	return true
}

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

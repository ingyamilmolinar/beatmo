package ui

import "math"

// TouchScroller tracks single-finger vertical scroll with momentum/inertia.
type TouchScroller struct {
	active    bool
	startX    int
	startY    int
	lastY     int
	velocity  float64 // pixels per frame
	dirLocked bool    // true once direction is committed
	isVert    bool    // committed direction (true = vertical)
}

const (
	touchScrollDeadZone = tapMaxMovePx // px before direction lock (matches gesture tap threshold)
	touchScrollFriction = 0.92         // velocity decay per frame
	touchScrollMinVel   = 0.5          // stop threshold
)

// Begin starts tracking from the given touch position.
func (ts *TouchScroller) Begin(x, y int) {
	ts.active = true
	ts.startX = x
	ts.startY = y
	ts.lastY = y
	ts.velocity = 0
	ts.dirLocked = false
	ts.isVert = false
}

// Move processes a touch move and returns the vertical pixel delta.
// Returns 0 if direction hasn't been committed yet or if horizontal.
func (ts *TouchScroller) Move(x, y int) float64 {
	if !ts.active {
		return 0
	}
	if !ts.dirLocked {
		dx := x - ts.startX
		dy := y - ts.startY
		// Per-axis dead zone, matching the gesture detector's tap threshold
		// (gesture.go: a touch is a TAP while dx <= tapMaxMovePx && dy <=
		// tapMaxMovePx). A Euclidean check here was STRICTER than that box —
		// e.g. an (8,8) real-finger jitter is Euclidean 11.3 (>10) yet per-axis
		// 8 (a tap) — so it committed a scroll and cancelled the deferred tap,
		// making instrument-menu items unselectable on real iPhone Safari while
		// open/scroll worked. Keeping the two thresholds identical means any
		// movement the gesture layer calls a tap never commits a scroll here.
		if abs(dx) <= touchScrollDeadZone && abs(dy) <= touchScrollDeadZone {
			return 0
		}
		ts.dirLocked = true
		ts.isVert = math.Abs(float64(dy)) >= math.Abs(float64(dx))
		if !ts.isVert {
			return 0
		}
		ts.lastY = y
		return 0
	}
	if !ts.isVert {
		return 0
	}
	delta := float64(y - ts.lastY)
	ts.velocity = delta // instantaneous velocity = last frame's delta
	ts.lastY = y
	return delta
}

// End ends the touch and returns the final velocity for momentum.
func (ts *TouchScroller) End() float64 {
	ts.active = false
	vel := ts.velocity
	// If direction was never locked or was horizontal, no momentum.
	if !ts.dirLocked || !ts.isVert {
		ts.velocity = 0
		return 0
	}
	return vel
}

// UpdateMomentum applies friction and returns the scroll delta for this frame.
// Call once per frame after the touch has ended.
func (ts *TouchScroller) UpdateMomentum() float64 {
	if math.Abs(ts.velocity) < touchScrollMinVel {
		ts.velocity = 0
		return 0
	}
	delta := ts.velocity
	ts.velocity *= touchScrollFriction
	return delta
}

// Active reports whether a touch is currently being tracked.
func (ts *TouchScroller) Active() bool { return ts.active }

// ScrollingCommitted reports whether the user has moved past the dead zone
// and committed to a vertical scroll. Before this point, the touch could be
// a tap on a button rather than a scroll gesture.
func (ts *TouchScroller) ScrollingCommitted() bool {
	return ts.active && ts.dirLocked && ts.isVert
}

// HasMomentum reports whether momentum scrolling is still in progress.
func (ts *TouchScroller) HasMomentum() bool {
	return !ts.active && math.Abs(ts.velocity) >= touchScrollMinVel
}

// Reset clears all state.
func (ts *TouchScroller) Reset() {
	*ts = TouchScroller{}
}

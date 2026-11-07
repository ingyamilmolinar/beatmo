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

// 0 until a vertical direction is committed.
func (ts *TouchScroller) Move(x, y int) float64 {
	if !ts.active {
		return 0
	}
	if !ts.dirLocked {
		dx := x - ts.startX
		dy := y - ts.startY
		// Per axis, matching the gesture detector's tap box exactly: a stricter Euclidean check committed
		// scrolls on finger jitter the detector called a tap, cancelling the tap.
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

// Once per frame after the touch ended.
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

// Before this, the touch may still be a tap.
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

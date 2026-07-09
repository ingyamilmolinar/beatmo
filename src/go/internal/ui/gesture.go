package ui

import (
	"math"
	"time"
)

// Gesture thresholds
const (
	longPressThresholdMS = 500 // ms to trigger long press
	tapMaxMovePx         = 10  // max movement for tap recognition
	tapMaxDurationMS     = 500 // max duration for tap
	pinchMinDistChange   = 20  // min distance change to trigger pinch
	panMinCenterMove     = 5   // min center movement for two-finger pan
)

// GestureKind identifies the type of detected gesture.
type GestureKind int

const (
	GestureNone GestureKind = iota
	GestureTap
	GestureLongPress
	GesturePinch
	GestureTwoFingerPan
	GestureSingleFingerDrag
)

// GestureEvent represents a detected gesture.
type GestureEvent struct {
	Kind GestureKind

	// Position for single-touch gestures (tap, long-press, single-finger drag)
	X, Y int

	// For pinch gesture
	CenterX, CenterY int
	Scale            float64 // ratio of current distance to start distance

	// For two-finger pan
	DeltaX, DeltaY int

	// For single-finger drag
	StartX, StartY int
}

// detectGesture analyzes the current touch state and returns a gesture event if detected.
func (ts *TouchState) detectGesture(endedTouches []*TouchPoint, now time.Time) *GestureEvent {
	touchCount := len(ts.points)

	// Handle touch end events
	if len(endedTouches) > 0 {
		// Reset multi-touch gesture state when fingers lift
		if touchCount < 2 {
			ts.isPinching = false
			ts.isPanning = false
			ts.twoFingerStarted = false
		}

		// Check for tap (single touch ended quickly with minimal movement).
		// Skip if multi-touch cooldown is active — the ended touch was the
		// trailing finger of a pinch/pan, not an intentional tap.
		if len(endedTouches) == 1 && touchCount == 0 && ts.multiTouchCooldown == 0 {
			pt := endedTouches[0]
			duration := now.Sub(pt.StartTime).Milliseconds()
			dx := abs(pt.X - pt.StartX)
			dy := abs(pt.Y - pt.StartY)

			if duration <= tapMaxDurationMS && dx <= tapMaxMovePx && dy <= tapMaxMovePx {
				return &GestureEvent{
					Kind: GestureTap,
					X:    pt.X,
					Y:    pt.Y,
				}
			}
		}
	}

	// Handle single touch
	if touchCount == 1 {
		pt := ts.PrimaryTouch()
		if pt == nil {
			return nil
		}

		// Check for long press (held in place) - fires exactly once per touch
		duration := now.Sub(pt.StartTime).Milliseconds()
		dx := abs(pt.X - pt.StartX)
		dy := abs(pt.Y - pt.StartY)

		if duration >= longPressThresholdMS && dx <= tapMaxMovePx && dy <= tapMaxMovePx && !pt.movedBeyondTap {
			// Only fire long-press once per touch sequence
			if !ts.longPressFired {
				ts.longPressFired = true
				return &GestureEvent{
					Kind: GestureLongPress,
					X:    pt.X,
					Y:    pt.Y,
				}
			}
			// Long-press already fired - fall through to check for drag
		}

		// Single finger drag (after initial threshold)
		// This is now reachable even after long-press fires, allowing drag-after-long-press
		if duration > 50 { // Small delay to distinguish from tap
			deltaX := pt.X - pt.PrevX
			deltaY := pt.Y - pt.PrevY
			if deltaX != 0 || deltaY != 0 {
				return &GestureEvent{
					Kind:   GestureSingleFingerDrag,
					X:      pt.X,
					Y:      pt.Y,
					StartX: pt.StartX,
					StartY: pt.StartY,
					DeltaX: deltaX,
					DeltaY: deltaY,
				}
			}
		}
	}

	// Handle two-finger gestures
	if touchCount == 2 {
		touches := ts.AllTouches()
		if len(touches) != 2 {
			return nil
		}

		t1, t2 := touches[0], touches[1]

		// Calculate current distance between fingers
		currDist := touchDistance(t1.X, t1.Y, t2.X, t2.Y)

		// Calculate centers
		currCX := (t1.X + t2.X) / 2
		currCY := (t1.Y + t2.Y) / 2
		prevCX := (t1.PrevX + t2.PrevX) / 2
		prevCY := (t1.PrevY + t2.PrevY) / 2

		// Initialize gesture tracking on first frame of two-finger touch
		if !ts.twoFingerStarted {
			ts.twoFingerStarted = true
			ts.pinchStartDist = currDist
			ts.pinchLastDist = currDist
			ts.panCenterX = currCX
			ts.panCenterY = currCY
			// Don't detect gestures on the initialization frame
			return nil
		}

		// Detect pinch vs pan
		distChange := math.Abs(currDist - ts.pinchLastDist)
		centerMoveX := abs(currCX - prevCX)
		centerMoveY := abs(currCY - prevCY)
		centerMove := centerMoveX + centerMoveY

		// Pinch takes priority if distance is changing significantly
		if distChange > float64(panMinCenterMove) {
			ts.isPinching = true
			ts.isPanning = false
			ts.pinchLastDist = currDist

			scale := currDist / ts.pinchStartDist
			if ts.pinchStartDist == 0 {
				scale = 1.0
			}

			return &GestureEvent{
				Kind:    GesturePinch,
				CenterX: currCX,
				CenterY: currCY,
				Scale:   scale,
			}
		}

		// Two-finger pan if center is moving
		if centerMove >= panMinCenterMove {
			ts.isPanning = true
			ts.isPinching = false

			return &GestureEvent{
				Kind:    GestureTwoFingerPan,
				CenterX: currCX,
				CenterY: currCY,
				DeltaX:  currCX - prevCX,
				DeltaY:  currCY - prevCY,
			}
		}
	}

	return nil
}

// touchDistance calculates the Euclidean distance between two points.
func touchDistance(x1, y1, x2, y2 int) float64 {
	dx := float64(x2 - x1)
	dy := float64(y2 - y1)
	return math.Sqrt(dx*dx + dy*dy)
}

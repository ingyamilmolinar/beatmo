package ui

import (
	"sync"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

// TouchPoint represents a single active touch.
type TouchPoint struct {
	ID        ebiten.TouchID
	X, Y      int       // current position
	StartX    int       // position at touch start
	StartY    int       // position at touch start
	StartTime time.Time // time when touch began
	PrevX     int       // previous frame position
	PrevY     int       // previous frame position

	// Latched once the touch moves past the tap threshold, so a slow drag cannot become a long-press
	// when a starved loop lets the 500 ms timer elapse mid-pan.
	movedBeyondTap bool
}

// TouchState tracks all active touches and gesture state.
type TouchState struct {
	points map[ebiten.TouchID]*TouchPoint

	// Gesture state
	isPinching       bool
	isPanning        bool
	twoFingerStarted bool // true once we've initialized for two-finger gesture
	pinchStartDist   float64
	pinchLastDist    float64
	panCenterX       int
	panCenterY       int

	// Long-press one-shot tracking: ensures long-press fires exactly once per touch
	longPressFired bool

	// So the finger that lifts last after a two-finger gesture cannot create a node.
	multiTouchCooldown int
}

const multiTouchCooldownFrames = 10

func (ts *TouchState) RecentMultiTouch() bool {
	return ts.multiTouchCooldown > 0
}

// NewTouchState creates a new touch state tracker.
func NewTouchState() *TouchState {
	return &TouchState{
		points: make(map[ebiten.TouchID]*TouchPoint),
	}
}

// globalTouchState is the singleton touch state used by the UI.
var globalTouchState = NewTouchState()

// TouchEventKind identifies the type of touch event.
type TouchEventKind int

const (
	TouchEventStart TouchEventKind = iota
	TouchEventMove
	TouchEventEnd
)

func (k TouchEventKind) String() string {
	switch k {
	case TouchEventStart:
		return "start"
	case TouchEventMove:
		return "move"
	case TouchEventEnd:
		return "end"
	default:
		return "unknown"
	}
}

// TouchDebugEvent records a single touch event for debugging.
type TouchDebugEvent struct {
	Timestamp time.Time
	Kind      TouchEventKind
	TouchID   int
	X, Y      int
}

// TouchDebugLog stores recent touch events for debugging.
type TouchDebugLog struct {
	mu       sync.RWMutex
	events   []TouchDebugEvent
	capacity int
	enabled  bool

	// Last detected gesture info
	lastGestureKind GestureKind
	lastGestureX    int
	lastGestureY    int
	lastGestureTime time.Time
}

const touchDebugLogCapacity = 50

// globalTouchDebugLog is the singleton debug log used by the UI.
var globalTouchDebugLog = &TouchDebugLog{
	events:   make([]TouchDebugEvent, 0, touchDebugLogCapacity),
	capacity: touchDebugLogCapacity,
	enabled:  false,
}

// SetTouchDebugEnabled enables or disables touch event logging.
func SetTouchDebugEnabled(enabled bool) {
	globalTouchDebugLog.mu.Lock()
	defer globalTouchDebugLog.mu.Unlock()
	globalTouchDebugLog.enabled = enabled
}

// IsTouchDebugEnabled returns whether touch event logging is enabled.
func IsTouchDebugEnabled() bool {
	globalTouchDebugLog.mu.RLock()
	defer globalTouchDebugLog.mu.RUnlock()
	return globalTouchDebugLog.enabled
}

// logTouchEvent records a touch event if debugging is enabled.
func logTouchEvent(kind TouchEventKind, id ebiten.TouchID, x, y int) {
	globalTouchDebugLog.mu.Lock()
	defer globalTouchDebugLog.mu.Unlock()

	if !globalTouchDebugLog.enabled {
		return
	}

	event := TouchDebugEvent{
		Timestamp: time.Now(),
		Kind:      kind,
		TouchID:   int(id),
		X:         x,
		Y:         y,
	}

	// Ring buffer behavior
	if len(globalTouchDebugLog.events) >= globalTouchDebugLog.capacity {
		// Shift events left (drop oldest)
		copy(globalTouchDebugLog.events, globalTouchDebugLog.events[1:])
		globalTouchDebugLog.events[len(globalTouchDebugLog.events)-1] = event
	} else {
		globalTouchDebugLog.events = append(globalTouchDebugLog.events, event)
	}
}

// logGestureEvent records a detected gesture for debugging.
func logGestureEvent(evt *GestureEvent) {
	if evt == nil || evt.Kind == GestureNone {
		return
	}

	globalTouchDebugLog.mu.Lock()
	defer globalTouchDebugLog.mu.Unlock()

	if !globalTouchDebugLog.enabled {
		return
	}

	globalTouchDebugLog.lastGestureKind = evt.Kind
	globalTouchDebugLog.lastGestureTime = time.Now()
	switch evt.Kind {
	case GestureTap, GestureLongPress, GestureSingleFingerDrag:
		globalTouchDebugLog.lastGestureX = evt.X
		globalTouchDebugLog.lastGestureY = evt.Y
	case GesturePinch, GestureTwoFingerPan:
		globalTouchDebugLog.lastGestureX = evt.CenterX
		globalTouchDebugLog.lastGestureY = evt.CenterY
	}
}

// TouchDebugSnapshot returns a snapshot of the current touch debug state.
type TouchDebugSnapshot struct {
	TouchCount      int
	Touches         []TouchDebugTouchInfo
	LastGestureKind string
	LastGestureX    int
	LastGestureY    int
	LastGestureAge  int64 // milliseconds since last gesture
	DPR             float64
	DebugEnabled    bool
}

// TouchDebugTouchInfo represents info about an active touch.
type TouchDebugTouchInfo struct {
	ID int
	X  int
	Y  int
}

// GetTouchDebugSnapshot returns current touch debug state.
func GetTouchDebugSnapshot() TouchDebugSnapshot {
	globalTouchDebugLog.mu.RLock()
	defer globalTouchDebugLog.mu.RUnlock()

	snap := TouchDebugSnapshot{
		TouchCount:   len(globalTouchState.points),
		Touches:      make([]TouchDebugTouchInfo, 0, len(globalTouchState.points)),
		DPR:          getDevicePixelRatio(),
		DebugEnabled: globalTouchDebugLog.enabled,
	}

	for id, pt := range globalTouchState.points {
		snap.Touches = append(snap.Touches, TouchDebugTouchInfo{
			ID: int(id),
			X:  pt.X,
			Y:  pt.Y,
		})
	}

	// Last gesture info
	switch globalTouchDebugLog.lastGestureKind {
	case GestureNone:
		snap.LastGestureKind = "none"
	case GestureTap:
		snap.LastGestureKind = "tap"
	case GestureLongPress:
		snap.LastGestureKind = "longPress"
	case GesturePinch:
		snap.LastGestureKind = "pinch"
	case GestureTwoFingerPan:
		snap.LastGestureKind = "twoFingerPan"
	case GestureSingleFingerDrag:
		snap.LastGestureKind = "singleFingerDrag"
	}
	snap.LastGestureX = globalTouchDebugLog.lastGestureX
	snap.LastGestureY = globalTouchDebugLog.lastGestureY
	if !globalTouchDebugLog.lastGestureTime.IsZero() {
		snap.LastGestureAge = time.Since(globalTouchDebugLog.lastGestureTime).Milliseconds()
	}

	return snap
}

// GetTouchEventLog returns a copy of all logged touch events.
func GetTouchEventLog() []TouchDebugEvent {
	globalTouchDebugLog.mu.RLock()
	defer globalTouchDebugLog.mu.RUnlock()

	result := make([]TouchDebugEvent, len(globalTouchDebugLog.events))
	copy(result, globalTouchDebugLog.events)
	return result
}

// ClearTouchEventLog clears the touch event log.
func ClearTouchEventLog() {
	globalTouchDebugLog.mu.Lock()
	defer globalTouchDebugLog.mu.Unlock()
	globalTouchDebugLog.events = globalTouchDebugLog.events[:0]
	globalTouchDebugLog.lastGestureKind = GestureNone
	globalTouchDebugLog.lastGestureX = 0
	globalTouchDebugLog.lastGestureY = 0
	globalTouchDebugLog.lastGestureTime = time.Time{}
}

// Overridden on WASM.
var getDevicePixelRatio = func() float64 {
	return 1.0
}

// ActiveTouchCount returns the number of currently active touches.
func (ts *TouchState) ActiveTouchCount() int {
	return len(ts.points)
}

// PrimaryTouch returns the first (oldest) active touch, or nil if none.
func (ts *TouchState) PrimaryTouch() *TouchPoint {
	if len(ts.points) == 0 {
		return nil
	}
	// Find the touch with the earliest start time
	var oldest *TouchPoint
	for _, pt := range ts.points {
		if oldest == nil || pt.StartTime.Before(oldest.StartTime) {
			oldest = pt
		}
	}
	return oldest
}

// GetTouch returns the touch point for the given ID, or nil if not found.
func (ts *TouchState) GetTouch(id ebiten.TouchID) *TouchPoint {
	return ts.points[id]
}

// AllTouches returns a slice of all active touch points.
func (ts *TouchState) AllTouches() []*TouchPoint {
	result := make([]*TouchPoint, 0, len(ts.points))
	for _, pt := range ts.points {
		result = append(result, pt)
	}
	return result
}

// Once per frame.
func (ts *TouchState) Update() *GestureEvent {
	ids := touchIDs()
	now := time.Now()

	// Track which touches are still active
	active := make(map[ebiten.TouchID]bool)
	for _, id := range ids {
		active[id] = true
	}

	// Decrement multi-touch cooldown each frame.
	if ts.multiTouchCooldown > 0 {
		ts.multiTouchCooldown--
	}

	// Process ended touches first
	var endedTouches []*TouchPoint
	for id, pt := range ts.points {
		if !active[id] {
			logTouchEvent(TouchEventEnd, id, pt.X, pt.Y)
			endedTouches = append(endedTouches, pt)
			delete(ts.points, id)
		}
	}

	if ts.twoFingerStarted && len(ts.points) < 2 && len(endedTouches) > 0 {
		ts.multiTouchCooldown = multiTouchCooldownFrames
	}

	// Reset long-press one-shot when all touches end
	if len(ts.points) == 0 && len(endedTouches) > 0 {
		ts.longPressFired = false
	}

	// Process new and updated touches
	for _, id := range ids {
		x, y := touchPosition(id)
		if pt, exists := ts.points[id]; exists {
			// Update existing touch
			if pt.X != x || pt.Y != y {
				logTouchEvent(TouchEventMove, id, x, y)
			}
			pt.PrevX, pt.PrevY = pt.X, pt.Y
			pt.X, pt.Y = x, y
			// >= (not >), so a finger sitting exactly on the boundary when the timer elapses is still disqualified.
			if abs(x-pt.StartX) >= tapMaxMovePx || abs(y-pt.StartY) >= tapMaxMovePx {
				pt.movedBeyondTap = true
			}
		} else {
			// New touch - reset long-press one-shot for new gesture sequence
			logTouchEvent(TouchEventStart, id, x, y)
			ts.longPressFired = false
			ts.points[id] = &TouchPoint{
				ID:        id,
				X:         x,
				Y:         y,
				StartX:    x,
				StartY:    y,
				StartTime: now,
				PrevX:     x,
				PrevY:     y,
			}
		}
	}

	// Detect and return gestures
	gesture := ts.detectGesture(endedTouches, now)
	logGestureEvent(gesture)
	return gesture
}

// Reset clears all touch state.
func (ts *TouchState) Reset() {
	ts.points = make(map[ebiten.TouchID]*TouchPoint)
	ts.isPinching = false
	ts.isPanning = false
	ts.twoFingerStarted = false
	ts.pinchStartDist = 0
	ts.pinchLastDist = 0
	ts.longPressFired = false
	ts.multiTouchCooldown = 0
}

// A single active touch overrides cursorPosition() and isMouseButtonPressed(), so every mouse
// handler works with touch.

var (
	touchOverrideActive bool
	touchOverrideX      int
	touchOverrideY      int
	touchOverrideLeft   bool

	// Held for the lift frame, so release handlers get the lift coords instead of the cursor's (0,0)
	// (a mid-drag slider would snap to an extreme).
	lastSingleTouchX    int
	lastSingleTouchY    int
	lastSingleTouchHeld bool // true when the next frame should publish the held lift coords
	pendingTouchRelease bool // true *during* the held release frame; cleared on the frame after

	// A tap fires after the touch ended, so a 2-frame press/release is injected for mouse handlers.
	touchTapInjected bool
	touchTapFrame    int // 0 = press frame, 1 = release frame
	touchTapX        int
	touchTapY        int
)

// Once per frame, after globalTouchState.Update() and before any cursor read.
func updateTouchOverride() {
	// Never interfere with test mocks.
	if inputForTestActive {
		touchOverrideActive = false
		lastSingleTouchHeld = false
		pendingTouchRelease = false
		return
	}

	if touchTapInjected {
		touchOverrideActive = true
		touchOverrideX = touchTapX
		touchOverrideY = touchTapY
		if touchTapFrame == 0 {
			// Frame 0: press
			touchOverrideLeft = true
			touchTapFrame = 1
		} else {
			// Frame 1: release — clear injection
			touchOverrideLeft = false
			touchTapInjected = false
		}
		// A tap injection supersedes any pending release-frame hold.
		lastSingleTouchHeld = false
		pendingTouchRelease = false
		return
	}

	// Not during the multi-touch cooldown, or the trailing finger looks like a fresh click.
	if globalTouchState.ActiveTouchCount() == 1 && !globalTouchState.RecentMultiTouch() {
		if pt := globalTouchState.PrimaryTouch(); pt != nil {
			touchOverrideActive = true
			touchOverrideX = pt.X
			touchOverrideY = pt.Y
			touchOverrideLeft = true
			lastSingleTouchX = pt.X
			lastSingleTouchY = pt.Y
			lastSingleTouchHeld = true
			pendingTouchRelease = false
			return
		}
	}

	// Publish the lift coords as released, so OnRelease gets the right position.
	if lastSingleTouchHeld {
		touchOverrideActive = true
		touchOverrideX = lastSingleTouchX
		touchOverrideY = lastSingleTouchY
		touchOverrideLeft = false
		lastSingleTouchHeld = false
		pendingTouchRelease = true
		return
	}

	// No override (post-release frame and beyond).
	pendingTouchRelease = false
	touchOverrideActive = false
}

// Taps fire after the touch ended, which the override cannot see: frame 0 press, frame 1 release at
// (x, y). Never during multi-touch.
func injectTouchTap(x, y int) {
	touchTapInjected = true
	touchTapFrame = 0
	touchTapX = x
	touchTapY = y
}

// Keeps the row scroller from recapturing an injected tap as a new scroll.
func isTouchTapInjecting() bool { return touchTapInjected }

// SetTouchTapInjectedForTest allows tests to simulate the tap injection state.
func SetTouchTapInjectedForTest(v bool) { touchTapInjected = v }

func SetTouchOverrideActiveForTest(v bool) { touchOverrideActive = v }

func SetTouchOverrideXYForTest(x, y int) {
	touchOverrideX = x
	touchOverrideY = y
}

// resetTouchOverride clears all touch override state (used in tests).
func resetTouchOverride() {
	touchOverrideActive = false
	touchOverrideX = 0
	touchOverrideY = 0
	touchOverrideLeft = false
	lastSingleTouchX = 0
	lastSingleTouchY = 0
	lastSingleTouchHeld = false
	pendingTouchRelease = false
	touchTapInjected = false
	touchTapFrame = 0
	touchTapX = 0
	touchTapY = 0
}

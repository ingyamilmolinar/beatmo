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

	// Multi-touch cooldown: after a two-finger gesture ends, suppress
	// single-touch override and tap detection for a few frames to prevent
	// accidental node creation when one finger lifts before the other.
	multiTouchCooldown int
}

// multiTouchCooldownFrames is the number of frames after a multi-touch gesture
// ends during which single-touch override and tap detection are suppressed.
const multiTouchCooldownFrames = 10

// RecentMultiTouch returns true if a multi-touch gesture ended recently
// and the cooldown has not yet expired.
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

// getDevicePixelRatio returns the device pixel ratio (implemented per platform).
// This is a placeholder that will be overridden on WASM.
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

// Update polls the current touch state from Ebiten and returns detected gesture
// events. Must be called once per frame.
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

	// Start multi-touch cooldown when a two-finger gesture ends
	// (fingers dropping from 2+ to <2).
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

// ─── Touch-to-mouse override ────────────────────────────────────────────────
// These variables let a single active touch transparently override
// cursorPosition() and isMouseButtonPressed() so that ALL existing
// mouse-based handlers (DrumView, Splitter, Camera, Editor, etc.)
// work automatically with touch input.

var (
	touchOverrideActive bool
	touchOverrideX      int
	touchOverrideY      int
	touchOverrideLeft   bool

	// Tap injection: when a tap gesture fires (touch already ended),
	// we inject a 2-frame press-then-release cycle so mouse handlers
	// see a click.
	touchTapInjected bool
	touchTapFrame    int // 0 = press frame, 1 = release frame
	touchTapX        int
	touchTapY        int
)

// updateTouchOverride sets the frame-level touch override state.
// Must be called once per frame, after globalTouchState.Update() and
// before any code reads cursorPosition/isMouseButtonPressed.
func updateTouchOverride() {
	// Never interfere with test mocks.
	if inputForTestActive {
		touchOverrideActive = false
		return
	}

	// Tap injection takes priority (touch already ended but we need
	// to synthesize a press+release for mouse handlers).
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
		return
	}

	// Single active touch maps to mouse — but not if a multi-touch gesture
	// just ended (cooldown prevents the trailing single finger from looking
	// like a fresh click to the editor).
	if globalTouchState.ActiveTouchCount() == 1 && !globalTouchState.RecentMultiTouch() {
		if pt := globalTouchState.PrimaryTouch(); pt != nil {
			touchOverrideActive = true
			touchOverrideX = pt.X
			touchOverrideY = pt.Y
			touchOverrideLeft = true
			return
		}
	}

	// No override.
	touchOverrideActive = false
}

// injectTouchTap starts a 2-frame tap injection cycle at (x, y).
//
// Why: taps fire after the touch has already ended (ActiveTouchCount==0), so the
// single-touch override can't see them. This function synthesizes:
//
//	Frame 0: position=(x,y), left=true  (press)
//	Frame 1: position=(x,y), left=false (release)
//
// Must NOT be called during multi-touch — only for single-finger taps in the
// drum area where the touch has ended before the gesture detector reports it.
func injectTouchTap(x, y int) {
	touchTapInjected = true
	touchTapFrame = 0
	touchTapX = x
	touchTapY = y
}

// isTouchTapInjecting returns true when a 2-frame tap injection cycle is active.
// Used to prevent the row touch scroller from recapturing an injected tap as a
// new scroll gesture (the gesture detector already confirmed this is a tap).
func isTouchTapInjecting() bool { return touchTapInjected }

// SetTouchTapInjectedForTest allows tests to simulate the tap injection state.
func SetTouchTapInjectedForTest(v bool) { touchTapInjected = v }

// SetTouchOverrideActiveForTest allows tests to simulate the touch override
// being active, indicating that the current frame's input originates from
// a hardware touch rather than a mouse.
func SetTouchOverrideActiveForTest(v bool) { touchOverrideActive = v }

// resetTouchOverride clears all touch override state (used in tests).
func resetTouchOverride() {
	touchOverrideActive = false
	touchOverrideX = 0
	touchOverrideY = 0
	touchOverrideLeft = false
	touchTapInjected = false
	touchTapFrame = 0
	touchTapX = 0
	touchTapY = 0
}

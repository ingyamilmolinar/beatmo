package ui

import "image"

// Every opaque zone registers this full-bounds catch-all at its nominal z, so a tap in its
// whitespace never falls through to a lower sibling. Consumed, not captured: no drag semantics, and
// per-control areas can still capture. Wheel is ignored so scrolling bubbles.

// Stateless, so one instance serves every catch-all.
type inputCaptureHandler struct{}

func (inputCaptureHandler) OnPress(x, y int) InputResult { return InputConsumed }

// A consumed press never receives drags.
func (inputCaptureHandler) OnDrag(x, y int) {}

// OnRelease is a no-op for the same reason.
func (inputCaptureHandler) OnRelease(x, y int) {}

// Ignored, so scrolling bubbles to the zone that owns it.
func (inputCaptureHandler) OnWheel(x, y, steps int) InputResult { return InputIgnored }

// File-level, so rebuildHitAreas never allocates one per frame.
var sharedInputCaptureHandler inputCaptureHandler

// The audio subtree is isolated with nothing scrollable beneath, so an unhandled wheel must stop
// here; bubbling would reach the legacy row-scroll fallback and scroll the drum rows.
type inputCaptureWheelHandler struct{ inputCaptureHandler }

func (inputCaptureWheelHandler) OnWheel(x, y, steps int) InputResult { return InputConsumed }

var sharedInputCaptureWheelHandler inputCaptureWheelHandler

// For opaque zones in an isolated subtree with no scroll owner beneath (the audio panel).
func NewInputCaptureHitAreaConsumeWheel(rect image.Rectangle, zIndex int, tag string) HitArea {
	return HitArea{
		Rect:    rect,
		ZIndex:  zIndex,
		Handler: sharedInputCaptureWheelHandler,
		Tag:     tag,
	}
}

// At the zone's nominal z; its controls sit at +1/+2 and win on overlap, so they stay interactive
// while the whitespace is opaque.
func NewInputCaptureHitArea(rect image.Rectangle, zIndex int, tag string) HitArea {
	return HitArea{
		Rect:    rect,
		ZIndex:  zIndex,
		Handler: sharedInputCaptureHandler,
		Tag:     tag,
	}
}

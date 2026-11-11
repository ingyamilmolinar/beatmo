package ui

// tapDebounce is the single shared primitive that de-bounces mobile taps. It is
// consulted at the one drum-area tap chokepoint (the injectTouchTap site in
// game_update.go) so EVERY mobile control reached by a tap — buttons, menu rows,
// and the value-wheel / other popup controls — shares the exact same guard.
//
// It is keyed on location + frame count: a tap is rejected when it lands within
// tapDebounceSlopPx of, and fewer than tapDebounceFrames after, the previous
// ACCEPTED tap. That catches hardware touch-bounce and an accidental fast
// double-tap on one control, while leaving legitimate quick taps on DIFFERENT
// controls (beyond the slop) untouched — the slop is far smaller than a touch
// target. Frame-count timing (advanced once per frame) matches the codebase
// idiom (multiTouchCooldownFrames, controlGridScrollCooldownFrames) and is
// deterministic under test.
//
// Tradeoff: a DELIBERATE re-tap of the SAME control faster than the window is
// also suppressed (e.g. re-tapping a synth card to toggle its popup closed
// within ~150ms — the popup can still be dismissed by tapping outside/center).
// The window is deliberately short so this only affects sub-window re-taps.
const (
	tapDebounceSlopPx = 12 // ≈ ¼ of a 44px touch target
	tapDebounceFrames = 9  // ≈ 150ms at 60fps
)

type tapDebounce struct {
	tick     int
	lastTick int
	lastX    int
	lastY    int
	has      bool
}

// advance ticks the debounce clock. Call exactly once per frame.
func (d *tapDebounce) advance() { d.tick++ }

// accept reports whether a tap at (x, y) should fire, recording it when it does.
// A tap within tapDebounceSlopPx and tapDebounceFrames of the previous accepted
// tap is rejected (returns false) and NOT recorded, so a sustained bounce burst
// keeps measuring from the first accepted tap rather than ratcheting forward.
func (d *tapDebounce) accept(x, y int) bool {
	if d.has && d.tick-d.lastTick < tapDebounceFrames {
		dx := x - d.lastX
		dy := y - d.lastY
		if dx*dx+dy*dy <= tapDebounceSlopPx*tapDebounceSlopPx {
			return false
		}
	}
	d.lastX, d.lastY, d.lastTick, d.has = x, y, d.tick, true
	return true
}

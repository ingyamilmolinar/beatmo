package ui

// Applied at the single drum-area tap chokepoint so every mobile control shares it. Rejects a tap within the slop
// and window of the last ACCEPTED one (bounce, accidental double-tap); a deliberate fast re-tap of one control is the cost.
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

// A rejected tap is not recorded, so a bounce burst keeps measuring from the first accepted tap.
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

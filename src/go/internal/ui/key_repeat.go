package ui

// The shared hold-to-repeat ACCELERATION RAMP; MenuKeyNav had no repeat while the curve lived inside Button. The
// firing rule is deliberately not shared: dueSince vs tickLegacyModulo.

// In FRAMES: Ebiten runs a fixed 60 TPS, so 30 frames = 0.5 s.
type repeatCurve struct {
	// The press edge itself is the caller's; this describes repeats only.
	Delay int
	// Interval is the starting gap between repeats.
	Interval int
	// AccelEvery is how often the gap shortens by one frame.
	AccelEvery int
	// A floor: an unclamped interval reaches 0 on a long hold and `step % 0` panics.
	MinInterval int
}

func (c repeatCurve) intervalAt(step int) int {
	if c.AccelEvery <= 0 {
		return c.Interval
	}
	interval := c.Interval - step/c.AccelEvery
	if min := c.MinInterval; interval < min {
		interval = min
	}
	if interval < 1 {
		interval = 1
	}
	return interval
}

// The menu rule: measured from the LAST fire, so the cadence is monotonic. The modulus rule double-fires at every
// acceleration step (gaps 6, 6, 2, 5), which stutters while steering a list.
func (c repeatCurve) dueSince(held, lastFired int) bool {
	if held <= c.Delay {
		return false
	}
	step := held - c.Delay
	if lastFired <= c.Delay {
		// No repeat yet this hold: the first one lands a full interval in.
		return step >= c.intervalAt(step)
	}
	return held-lastFired >= c.intervalAt(step)
}

// Kept exactly for the BPM and length +/- buttons: its double-fire is imperceptible while scrubbing a number and
// "fixing" it would change their feel. New callers use dueSince.
func (c repeatCurve) tickLegacyModulo(held int) bool {
	if held <= c.Delay {
		return false
	}
	step := held - c.Delay
	return step%c.intervalAt(step) == 0
}

var (
	// 1 s before it starts, 10/s accelerating to 60/s: a number wants to fly.
	buttonRepeat = repeatCurve{Delay: 60, Interval: 6, AccelEvery: 30, MinInterval: 1}

	// Gentler (0.5 s, 10 rows/s to 30): a list is STEERED; at 60 rows/s the whole roster passes in two seconds.
	menuRepeat = repeatCurve{Delay: 30, Interval: 6, AccelEvery: 20, MinInterval: 2}
)

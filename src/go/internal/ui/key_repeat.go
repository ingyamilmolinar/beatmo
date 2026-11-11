package ui

// key_repeat.go — the shared hold-to-repeat cadence.
//
// This curve lived inline in Button.repeatTick (uigrid.go) and was reachable by
// nothing else, which is why MenuKeyNav had no repeat at all: a held arrow moved
// exactly one row. Copying the formula into the nav would have been a second
// implementation free to drift — the parallel-implementations pattern that
// produced the platform wheel bug.
//
// What is genuinely shared is the ACCELERATION RAMP (intervalAt). The FIRING
// RULE is not, and deliberately so — see dueSince vs tickLegacyModulo below.

// repeatCurve describes a hold-to-repeat cadence in FRAMES. Ebiten runs a fixed
// 60 TPS, so a frame count is a time: 30 frames == 0.5s.
type repeatCurve struct {
	// Delay is the frames held before the FIRST repeat. The press edge itself is
	// the caller's business — this curve only describes repeats.
	Delay int
	// Interval is the starting gap between repeats.
	Interval int
	// AccelEvery is how often the gap shortens by one frame.
	AccelEvery int
	// MinInterval is the fastest gap. It is a floor, not a suggestion: an
	// unclamped interval reaches 0 on a long hold and `step % 0` panics.
	MinInterval int
}

// intervalAt is the ramp: the gap between repeats `step` frames after the delay
// expired. This is the part both callers share.
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

// dueSince reports whether a control held for `held` frames, last fired on frame
// `lastFired`, fires now. This is the rule MENUS use.
//
// It measures the gap from the LAST FIRE rather than testing the absolute frame
// against a modulus, which is what makes the cadence monotonic. The modulus rule
// (tickLegacyModulo) double-fires at every acceleration step: with the menu
// tuning it produces gaps of 6, 6, 2, 5 — two rows jumping almost together and
// then a pause, which reads as a stutter while steering a list.
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

// tickLegacyModulo is the ORIGINAL Button.repeatTick rule, preserved exactly.
//
// It is kept because the BPM and timeline-length +/- buttons have shipped with
// this cadence — including its double-fire at each acceleration step, which is
// imperceptible while scrubbing a number and would be a gratuitous change in
// feel to "fix". New callers should use dueSince.
func (c repeatCurve) tickLegacyModulo(held int) bool {
	if held <= c.Delay {
		return false
	}
	step := held - c.Delay
	return step%c.intervalAt(step) == 0
}

var (
	// buttonRepeat is the cadence the BPM and timeline-length +/- buttons have
	// always had: 1.0s before it starts, 10/s accelerating to 60/s. Scrubbing a
	// NUMBER wants to fly, so the floor is one frame.
	buttonRepeat = repeatCurve{Delay: 60, Interval: 6, AccelEvery: 30, MinInterval: 1}

	// menuRepeat is deliberately gentler: 0.5s before it starts, 10 rows/s
	// accelerating to 30 rows/s. A list is STEERED rather than scrubbed — at the
	// button curve's 60 rows/second the whole 133-instrument roster passes in
	// about two seconds, which cannot be aimed.
	menuRepeat = repeatCurve{Delay: 30, Interval: 6, AccelEvery: 20, MinInterval: 2}
)

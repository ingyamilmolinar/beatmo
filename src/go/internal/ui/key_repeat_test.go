//go:build test

// The shared hold-to-repeat cadence.
//
// This curve was previously inlined in Button.repeatTick and available to
// nothing else, so MenuKeyNav had no repeat at all — a held arrow moved exactly
// one row. Extracting it gives both a single implementation with per-caller
// tuning, rather than a second copy that drifts (the parallel-implementations
// pattern that produced the platform wheel bug).

package ui

import "testing"

// ticksIn returns the frames (relative to press) on which the curve fires under
// the MENU rule, simulating a key held continuously for n frames — including the
// lastFired bookkeeping a real caller does.
func ticksIn(c repeatCurve, n int) []int {
	var out []int
	lastFired := 0
	for held := 1; held <= n; held++ {
		if c.dueSince(held, lastFired) {
			out = append(out, held)
			lastFired = held
		}
	}
	return out
}

func TestRepeatCurve_SilentUntilTheDelayElapses(t *testing.T) {
	c := repeatCurve{Delay: 30, Interval: 6, AccelEvery: 20, MinInterval: 2}
	for held := 1; held <= 30; held++ {
		if c.dueSince(held, 0) {
			t.Fatalf("curve fired at held=%d, inside the %d-frame delay — a brief "+
				"hold must move exactly one row", held, c.Delay)
		}
	}
}

func TestRepeatCurve_FirstRepeatLandsOneIntervalAfterTheDelay(t *testing.T) {
	c := repeatCurve{Delay: 30, Interval: 6, AccelEvery: 20, MinInterval: 2}
	ticks := ticksIn(c, 40)
	if len(ticks) == 0 {
		t.Fatal("curve never fired within 40 frames")
	}
	if got, want := ticks[0], c.Delay+c.Interval; got != want {
		t.Errorf("first repeat at frame %d, want %d (delay + one interval)", got, want)
	}
}

func TestRepeatCurve_Accelerates(t *testing.T) {
	c := repeatCurve{Delay: 30, Interval: 6, AccelEvery: 20, MinInterval: 2}
	ticks := ticksIn(c, 300)
	if len(ticks) < 10 {
		t.Fatalf("only %d repeats in 300 frames", len(ticks))
	}
	var gaps []int
	for i := 1; i < len(ticks); i++ {
		gaps = append(gaps, ticks[i]-ticks[i-1])
	}
	for i := 1; i < len(gaps); i++ {
		if gaps[i] > gaps[i-1] {
			t.Errorf("cadence SLOWED at repeat %d: gap %d then %d (gaps=%v) — the "+
				"curve must be monotonically non-increasing", i, gaps[i-1], gaps[i], gaps)
			break
		}
	}
	if first, last := gaps[0], gaps[len(gaps)-1]; last >= first {
		t.Errorf("no acceleration: first gap %d, last gap %d", first, last)
	}
}

func TestRepeatCurve_NeverExceedsMinInterval(t *testing.T) {
	c := repeatCurve{Delay: 30, Interval: 6, AccelEvery: 20, MinInterval: 2}
	ticks := ticksIn(c, 1000)
	for i := 1; i < len(ticks); i++ {
		if gap := ticks[i] - ticks[i-1]; gap < c.MinInterval {
			t.Fatalf("gap %d at repeat %d is below MinInterval %d — an unclamped "+
				"interval reaching 0 would divide by zero or fire every frame",
				gap, i, c.MinInterval)
		}
	}
}

// A long hold must not panic or fire every frame once the accelerator would
// drive the interval past zero. `6 - step/20` goes negative at step 140.
func TestRepeatCurve_SurvivesAVeryLongHold(t *testing.T) {
	c := repeatCurve{Delay: 30, Interval: 6, AccelEvery: 20, MinInterval: 2}
	lastFired := 0
	for held := 1; held <= 10000; held++ {
		if c.dueSince(held, lastFired) { // must not panic (divide/modulo by zero)
			lastFired = held
		}
		c.tickLegacyModulo(held)
	}
}

// TestButtonRepeatCurveIsUnchanged pins the EXACT cadence the BPM and timeline
// +/- buttons have today. The extraction must be behaviour-preserving: this
// compares the shared curve against the original inline formula, frame by frame.
func TestButtonRepeatCurveIsUnchanged(t *testing.T) {
	original := func(d int) bool {
		if d <= 60 {
			return false
		}
		step := d - 60
		accel := step / 30
		if accel > 5 {
			accel = 5
		}
		return step%(6-accel) == 0
	}
	for held := 1; held <= 2000; held++ {
		if got, want := buttonRepeat.tickLegacyModulo(held), original(held); got != want {
			t.Fatalf("held=%d: shared curve says %v, original Button.repeatTick says %v "+
				"— the extraction changed BPM/timeline +/- feel", held, got, want)
		}
	}
}

// The menu curve is deliberately gentler than the button curve: a list is
// steered, a number is scrubbed.
func TestMenuRepeatCurveIsGentlerThanTheButtonCurve(t *testing.T) {
	if menuRepeat.Delay >= buttonRepeat.Delay {
		t.Errorf("menu delay %d is not shorter than the button delay %d — a menu "+
			"should start repeating sooner", menuRepeat.Delay, buttonRepeat.Delay)
	}
	if menuRepeat.MinInterval <= buttonRepeat.MinInterval {
		t.Errorf("menu MinInterval %d is not gentler than the button's %d — a list "+
			"flying at 60 rows/second cannot be steered",
			menuRepeat.MinInterval, buttonRepeat.MinInterval)
	}
}

// TestMenuRuleHasNoDoubleFireAtAccelerationSteps is why menus do not use the
// legacy modulus rule.
//
// `step % interval` re-tests the ABSOLUTE frame against a modulus that changes
// mid-hold, so the frame an acceleration step lands on often satisfies both the
// old and the new modulus — firing twice in quick succession. Harmless while
// scrubbing a number; while steering a list it is a visible stutter, two rows
// jumping together and then a pause.
func TestMenuRuleHasNoDoubleFireAtAccelerationSteps(t *testing.T) {
	legacy := func(c repeatCurve, n int) []int {
		var out []int
		for held := 1; held <= n; held++ {
			if c.tickLegacyModulo(held) {
				out = append(out, held)
			}
		}
		return out
	}
	gaps := func(ticks []int) []int {
		var out []int
		for i := 1; i < len(ticks); i++ {
			out = append(out, ticks[i]-ticks[i-1])
		}
		return out
	}
	// stutters reports a gap that is SHORTER than the one after it — the
	// signature of a double-fire: two rows jump together, then a pause.
	stutters := func(g []int) bool {
		for i := 1; i < len(g); i++ {
			if g[i] > g[i-1] {
				return true
			}
		}
		return false
	}

	legacyGaps := gaps(legacy(menuRepeat, 300))
	if !stutters(legacyGaps) {
		t.Fatalf("harness: the legacy modulus rule was expected to stutter with the "+
			"menu tuning, but its cadence was monotonic: %v", legacyGaps)
	}
	menuGaps := gaps(ticksIn(menuRepeat, 300))
	if stutters(menuGaps) {
		t.Errorf("the menu rule stutters: %v — a gap shorter than the one after it "+
			"means it double-fired at an acceleration step", menuGaps)
	}
}

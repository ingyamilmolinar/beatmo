package ui

import (
	"image"
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// TestStepLadderIncludesUnitaryResolution guards that the resolution strip can
// ALWAYS be set to a human "unitary" resolution — a power-of-ten step such as 1%
// or 0.1% (…0.1, 1, 10…). The bug: for a percentage control the ladder was
// {0.05, 0.5, 2, 10}%, jumping 0.5% → 2% and skipping the natural 1% tick.
func TestStepLadderIncludesUnitaryResolution(t *testing.T) {
	cases := []struct {
		def  audio.ParamDef
		want float64 // the "1%" step, in the param's real units
	}{
		// Dimensionless 0..1 (shown as a percentage): 1% == 0.01 real units.
		{audio.ParamDef{Name: "amount", Min: 0, Max: 1}, 0.01},
		// A 0..100 percent control: 1% == 1.
		{audio.ParamDef{Name: "mix", Min: 0, Max: 100, Unit: "%"}, 1},
		// The real transient attack/sustain params (0..200 %): 1% == 1.
		{audio.ParamDef{Name: "attack", Min: 0, Max: 200, Unit: "%"}, 1},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.def.Name, func(t *testing.T) {
			ladder := stepLadder(tc.def)
			for _, s := range ladder {
				if math.Abs(s-tc.want) <= tc.want*1e-9 {
					return // unitary rung present
				}
			}
			t.Fatalf("ladder %v does not include the unitary resolution %v", ladder, tc.want)
		})
	}
}

// TestStepLadderRungsAreConsecutiveNiceSteps guards that the ticks make sense for
// humans: every adjacent pair is a CONSECUTIVE step in the 1/2/5×10^k sequence,
// so the ratio is exactly 2 (1→2, 5→10) or 2.5 (2→5). A larger ratio means a
// human-meaningful rung (like the unitary 1×10^k) was skipped.
func TestStepLadderRungsAreConsecutiveNiceSteps(t *testing.T) {
	defs := []audio.ParamDef{
		{Name: "amount", Min: 0, Max: 1},
		{Name: "mix", Min: 0, Max: 100, Unit: "%"},
		{Name: "attack", Min: 0, Max: 200, Unit: "%"},
		{Name: "cutoff", Min: 20, Max: 20000, Unit: "Hz"},
		{Name: "decay", Min: 0, Max: 4},
	}
	for _, def := range defs {
		def := def
		t.Run(def.Name, func(t *testing.T) {
			ladder := stepLadder(def)
			for i := 1; i < len(ladder); i++ {
				ratio := ladder[i] / ladder[i-1]
				if math.Abs(ratio-2) > 1e-6 && math.Abs(ratio-2.5) > 1e-6 {
					t.Fatalf("non-consecutive rungs %v -> %v (ratio %v) skips a nice-step; ladder %v",
						ladder[i-1], ladder[i], ratio, ladder)
				}
			}
		})
	}
}

func TestNiceStepRounds(t *testing.T) {
	// niceStep rounds x UP to the nearest value in the 1/2/5 × 10^k sequence.
	// Examples:
	//   0.011 → the sequence near 0.01 is 0.01, 0.02, 0.05, 0.1 → first ≥ 0.011 is 0.02
	//   0.04  → first ≥ 0.04 in sequence is 0.05
	//   0.4   → first ≥ 0.4 in sequence is 0.5
	//   1.1   → first ≥ 1.1 in sequence is 2
	//   3     → first ≥ 3 in sequence is 5
	//   7     → first ≥ 7 in sequence is 10
	//   9     → first ≥ 9 in sequence is 10
	//   90    → first ≥ 90 in sequence is 100
	cases := map[float64]float64{
		0.011: 0.02,
		0.04:  0.05,
		0.4:   0.5,
		1.1:   2,
		3:     5,
		7:     10,
		9:     10,
		90:    100,
	}
	for in, want := range cases {
		if got := niceStep(in); got != want {
			t.Fatalf("niceStep(%v)=%v want %v", in, got, want)
		}
	}
}

func TestStepLadderForHz(t *testing.T) {
	def := audio.ParamDef{Name: "filter_cutoff", Min: 20, Max: 20000, Unit: "Hz"}
	ladder := stepLadder(def)
	if len(ladder) < 2 {
		t.Fatalf("ladder too short: %v", ladder)
	}
	for i := 1; i < len(ladder); i++ {
		if ladder[i] <= ladder[i-1] {
			t.Fatalf("ladder not strictly increasing: %v", ladder)
		}
	}
	idx := defaultStepIndex(def, ladder)
	if idx < 0 || idx >= len(ladder) {
		t.Fatalf("defaultStepIndex out of range: %d (len %d)", idx, len(ladder))
	}
	notches := (def.Max - def.Min) / ladder[idx]
	if notches < 100 || notches > 2000 {
		t.Fatalf("default rung notches=%v out of nuanced band [100,2000]", notches)
	}
}

func TestStepLadderDegenerateRange(t *testing.T) {
	// Zero/negative span must not panic and must yield a usable ladder.
	def := audio.ParamDef{Name: "x", Min: 1, Max: 1}
	ladder := stepLadder(def)
	if len(ladder) < 1 {
		t.Fatalf("ladder empty for degenerate range")
	}
	if idx := defaultStepIndex(def, ladder); idx < 0 || idx >= len(ladder) {
		t.Fatalf("defaultStepIndex bad: %d", idx)
	}
}

// Unit-carrying steps render with a "±" prefix so the badge/wheel chip
// reads as a STEP SIZE, not a second value readout ("50 Hz" under a knob
// showing "8.00 kHz" read as another value — 2026-07-04 critique D-item).
// Unit-less steps keep the "x" multiplier prefix, which already signals
// "not a value".
func TestFormatStepValue(t *testing.T) {
	if got := formatStepValue(audio.ParamDef{Unit: "Hz"}, 100); got != "±100 Hz" {
		t.Fatalf("got %q want \"±100 Hz\"", got)
	}
	if got := formatStepValue(audio.ParamDef{Unit: ""}, 0.1); got != "x0.1" {
		t.Fatalf("got %q want \"x0.1\"", got)
	}
}

func TestTrimFloat(t *testing.T) {
	if got := trimFloat(0.5); got != "0.5" {
		t.Fatalf("got %q", got)
	}
	if got := trimFloat(100); got != "100" {
		t.Fatalf("got %q", got)
	}
}

func TestKnobStepBadgeCyclesAndReports(t *testing.T) {
	def := audio.ParamDef{Name: "filter_cutoff", Min: 20, Max: 20000, Unit: "Hz"}
	b := NewKnobStepBadge(def)
	first := b.Step()
	if first <= 0 {
		t.Fatalf("step not positive: %v", first)
	}
	n := b.LadderLen()
	for i := 0; i < n; i++ {
		b.Cycle()
	}
	if b.Step() != first {
		t.Fatalf("cycling %d times must return to start; got %v want %v", n, b.Step(), first)
	}
	if b.Label() == "" {
		t.Fatalf("label empty")
	}
}

func TestKnobStepBadgeAdjustResolution(t *testing.T) {
	def := audio.ParamDef{Name: "filter_cutoff", Min: 20, Max: 20000, Unit: "Hz"}
	b := NewKnobStepBadge(def)
	n := b.LadderLen()
	if n < 2 {
		t.Fatalf("need >=2 rungs, got %d", n)
	}
	// Scroll up (positive = finer) repeatedly clamps at the finest (smallest step).
	for i := 0; i < n+3; i++ {
		b.AdjustResolution(1)
	}
	finest := b.Step()
	if b.AdjustResolution(1) {
		t.Fatalf("AdjustResolution past finest should clamp (no change)")
	}
	// Scroll down (negative = coarser) repeatedly clamps at the coarsest (largest step).
	for i := 0; i < n+3; i++ {
		b.AdjustResolution(-1)
	}
	coarsest := b.Step()
	if b.AdjustResolution(-1) {
		t.Fatalf("AdjustResolution past coarsest should clamp (no change)")
	}
	if finest >= coarsest {
		t.Fatalf("finest step (%v) must be smaller than coarsest (%v)", finest, coarsest)
	}
}

func TestKnobStepBadgeWheelResolutionIsClicky(t *testing.T) {
	def := audio.ParamDef{Name: "filter_cutoff", Min: 20, Max: 20000, Unit: "Hz"}
	b := NewKnobStepBadge(def)
	start := b.Step()
	// Fewer than the per-rung threshold of notches: no rung change yet.
	for i := 0; i < knobBadgeWheelNotchesPerRung-1; i++ {
		if b.WheelResolution(-1) {
			t.Fatalf("rung moved before notch threshold (notch %d)", i+1)
		}
	}
	if b.Step() != start {
		t.Fatalf("step changed before threshold: %v -> %v", start, b.Step())
	}
	// Crossing the threshold advances exactly one rung (coarser).
	if !b.WheelResolution(-1) {
		t.Fatalf("rung should move once the notch threshold is crossed")
	}
	if b.Step() <= start {
		t.Fatalf("expected one coarser rung: %v -> %v", start, b.Step())
	}
	// A single large-magnitude event counts as ONE notch, never a multi-rung jump.
	b2 := NewKnobStepBadge(def)
	s2 := b2.Step()
	if b2.WheelResolution(-50) {
		t.Fatalf("a single high-magnitude wheel event must not jump a rung")
	}
	if b2.Step() != s2 {
		t.Fatalf("single big event changed the step: %v -> %v", s2, b2.Step())
	}
}

func TestKnobStepBadgeSetStepSnapsToNearestRung(t *testing.T) {
	def := audio.ParamDef{Name: "x", Min: 0, Max: 1000, Unit: ""}
	b := NewKnobStepBadge(def)
	coarsest := b.Step()
	for i := 0; i < b.LadderLen(); i++ { // advance to the coarsest rung to capture it
		if b.Step() > coarsest {
			coarsest = b.Step()
		}
		b.Cycle()
	}
	b.SetStep(coarsest * 1.01) // slightly above coarsest must snap to coarsest
	if b.Step() != coarsest {
		t.Fatalf("SetStep snapped to %v want coarsest %v", b.Step(), coarsest)
	}
}

func TestKnobStepBadgeRectRoundTrips(t *testing.T) {
	b := NewKnobStepBadge(audio.ParamDef{Name: "x", Min: 0, Max: 100})
	r := image.Rect(1, 2, 40, 20)
	b.SetRect(r)
	if b.Rect() != r {
		t.Fatalf("rect=%v want %v", b.Rect(), r)
	}
}

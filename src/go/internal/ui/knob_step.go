package ui

import (
	"image"
	"math"
	"strconv"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// niceStep rounds x UP to the nearest value in the 1/2/5 × 10^k sequence.
//
// The 1/2/5 decade sequence produces human-friendly step sizes because each
// rung is easily divisible and visually meaningful on a scale. For example:
//
//	…  0.01  0.02  0.05  0.1  0.2  0.5  1  2  5  10  20  50  100  …
//
// Given x, niceStep finds the decade k such that 10^k ≤ x < 10^(k+1), then
// returns the first element of {1, 2, 5, 10} × 10^k that is ≥ x/10^k scaled
// back up:
//
//	f = x / 10^k  (so 1 ≤ f < 10)
//	  f ∈ (0,1]  → 1 × 10^k   (x is exactly at decade boundary)
//	  f ∈ (1,2]  → 2 × 10^k
//	  f ∈ (2,5]  → 5 × 10^k
//	  f ∈ (5,10) → 10 × 10^k
//
// niceStep returns 1 for x ≤ 0.
func niceStep(x float64) float64 {
	if x <= 0 {
		return 1
	}
	exp := math.Floor(math.Log10(x))
	pow := math.Pow(10, exp)
	f := x / pow
	switch {
	case f <= 1:
		return pow
	case f <= 2:
		return 2 * pow
	case f <= 5:
		return 5 * pow
	default:
		return 10 * pow
	}
}

// nextNiceStep returns the next-larger value in the 1/2/5 × 10^k sequence after
// x (which is assumed to already be a nice-step): 1→2, 2→5, 5→10. This walks the
// sequence one rung at a time so a ladder built from it never skips a rung.
func nextNiceStep(x float64) float64 {
	if x <= 0 {
		return 1
	}
	exp := math.Floor(math.Log10(x) + 1e-9)
	pow := math.Pow(10, exp)
	m := x / pow // ≈ 1, 2, or 5
	switch {
	case m < 1.5:
		return 2 * pow
	case m < 3.5:
		return 5 * pow
	default:
		return 10 * pow
	}
}

// stepLadder builds a strictly-increasing slice of "nice" step sizes for def,
// ranging from a fine rung (~span/2000 notches) to a coarse rung (~span/16
// notches). The rungs are the CONSECUTIVE run of 1/2/5 × 10^k nice-steps between
// those endpoints — never a sparse sample — so the resolution strip always
// exposes the human "unitary" resolutions (…0.1, 1, 10…, e.g. 1%) and never
// jumps 0.5% → 2%, which is not a sensible tick for a human.
//
// For a degenerate range (span ≤ 0) a single-element ladder {1} is returned.
func stepLadder(def audio.ParamDef) []float64 {
	span := def.Max - def.Min
	if span <= 0 {
		return []float64{1}
	}
	fine := niceStep(span / 2000) // ~2000 notches across the range (fine)
	coarse := niceStep(span / 16) // ~16 notches across the range (coarse)
	if coarse < fine {
		coarse = fine
	}
	out := []float64{fine}
	for s := nextNiceStep(fine); s <= coarse*(1+1e-9); s = nextNiceStep(s) {
		out = append(out, s)
		if len(out) >= 24 { // safety bound; the fine→coarse span (~125×) is ~8 rungs
			break
		}
	}
	return out
}

// defaultStepNotches is the detent count a control's STARTING resolution aims
// for: enough to sweep the range in a couple of hundred clicks without making a
// single click imperceptible.
const defaultStepNotches = 200

// defaultStepIndex returns the index into ladder the control should START on:
// the ROUND DECADE rung (a pure 1×10^k — …0.01, 0.1, 1, 10, 100…) closest to
// span/defaultStepNotches. Picking the decade rung is what makes a control's
// default step a number a human would count by — ±1 dB, ±100 Hz, ±0.01 s —
// rather than whatever odd rung (±0.2 dB, ±50 Hz, ±0.005 s) happened to sit
// nearest a raw detent-count target.
//
// Ladders are logarithmically spaced, so "closest" is measured as a RATIO, not
// an absolute difference: against a target of 0.36, the rung 1 (ratio 2.8) beats
// the rung 0.1 (ratio 3.6) even though 0.1 is nearer on a number line.
//
// A ladder with no decade rung at all (possible only for a hand-built ladder —
// stepLadder always walks whole 1/2/5 decades) falls back to the nearest rung by
// the same ratio measure. The result is always a valid index into ladder.
func defaultStepIndex(def audio.ParamDef, ladder []float64) int {
	span := def.Max - def.Min
	if span <= 0 || len(ladder) == 0 {
		return 0
	}
	target := span / defaultStepNotches
	decade, decadeErr := -1, math.Inf(1)
	nearest, nearestErr := 0, math.Inf(1)
	for i, s := range ladder {
		e := stepRatioDistance(s, target)
		if e < nearestErr {
			nearest, nearestErr = i, e
		}
		if isDecadeStep(s) && e < decadeErr {
			decade, decadeErr = i, e
		}
	}
	if decade >= 0 {
		return decade
	}
	return nearest
}

// isDecadeStep reports whether s is a pure power of ten (mantissa 1) — the
// "round" rungs of the 1/2/5 ladder.
func isDecadeStep(s float64) bool {
	if s <= 0 {
		return false
	}
	l := math.Log10(s)
	return math.Abs(l-math.Round(l)) < 1e-9
}

// stepRatioDistance measures how far apart two step sizes are on a log scale:
// the larger over the smaller, so 1 means identical and the measure is
// symmetric. +Inf for non-positive input.
func stepRatioDistance(a, b float64) float64 {
	if a <= 0 || b <= 0 {
		return math.Inf(1)
	}
	if a < b {
		a, b = b, a
	}
	return a / b
}

// formatStepValue formats a step size for display. When def carries a unit
// (e.g. "Hz", "ms", "dB") the result is "±100 Hz" — the "±" marks the chip
// as a STEP SIZE so it never reads as a second value readout next to the
// knob's actual value (like ÷n, ± is a math operator; see the DESIGN.md
// permitted-text-glyph table). For dimensionless params the result is
// "x0.1" — the ASCII letter x, NOT the multiplication glyph, to satisfy
// the glyph lint on DESIGN.md forbidden-glyph list.
func formatStepValue(def audio.ParamDef, step float64) string {
	s := trimFloat(step)
	if def.Unit == "" {
		return "x" + s
	}
	return "±" + s + " " + def.Unit
}

// trimFloat formats v as a decimal string with no trailing zeros or
// unnecessary decimal point (e.g. 100 → "100", 0.5 → "0.5").
func trimFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// KnobStepBadge is the small tappable pill drawn under a continuous knob
// showing the current step (real units, e.g. "100 Hz"). Tapping cycles a
// fixed ladder of rungs. The chosen rung drives the knob's StepMul and is
// persisted by param name.
type KnobStepBadge struct {
	def    audio.ParamDef
	ladder []float64
	idx    int
	r      image.Rectangle
}

// NewKnobStepBadge creates a badge for the given param, starting at the
// default rung for that param's range.
func NewKnobStepBadge(def audio.ParamDef) *KnobStepBadge {
	l := stepLadder(def)
	return &KnobStepBadge{def: def, ladder: l, idx: defaultStepIndex(def, l)}
}

// Step returns the currently selected step in real param units.
func (b *KnobStepBadge) Step() float64 {
	if b.idx < 0 || b.idx >= len(b.ladder) {
		return 1
	}
	return b.ladder[b.idx]
}

// LadderLen returns the number of rungs.
func (b *KnobStepBadge) LadderLen() int { return len(b.ladder) }

// SetIndex jumps directly to ladder rung i (clamped). Returns true if moved.
func (b *KnobStepBadge) SetIndex(i int) bool {
	if len(b.ladder) == 0 {
		return false
	}
	if i < 0 {
		i = 0
	} else if i >= len(b.ladder) {
		i = len(b.ladder) - 1
	}
	if i == b.idx {
		return false
	}
	b.idx = i
	return true
}

// Index returns the current ladder rung index.
func (b *KnobStepBadge) Index() int { return b.idx }

// Rungs returns the ladder steps (fine→coarse). Read-only use.
func (b *KnobStepBadge) Rungs() []float64 { return b.ladder }

// AdjustResolution shifts the rung by delta where a POSITIVE delta means finer
// resolution (a smaller step). The ladder is ordered fine→coarse (index 0 is
// the smallest step), so finer == lower index. Clamps at the ends (no wrap, so
// the mouse wheel feels like a bounded range). Returns true if the rung moved.
func (b *KnobStepBadge) AdjustResolution(delta int) bool {
	if len(b.ladder) == 0 {
		return false
	}
	newIdx := b.idx - delta // finer (delta>0) => lower index => smaller step
	if newIdx < 0 {
		newIdx = 0
	} else if newIdx > len(b.ladder)-1 {
		newIdx = len(b.ladder) - 1
	}
	if newIdx == b.idx {
		return false
	}
	b.idx = newIdx
	return true
}

// Label is the on-pill text for the current step.
func (b *KnobStepBadge) Label() string { return formatStepValue(b.def, b.Step()) }

// ParamName returns the param this badge controls (used for persistence).
func (b *KnobStepBadge) ParamName() string { return b.def.Name }

// SetRect sets the pill's bounding rectangle for layout and draw.
func (b *KnobStepBadge) SetRect(r image.Rectangle) { b.r = r }

// Rect returns the pill's bounding rectangle.
func (b *KnobStepBadge) Rect() image.Rectangle { return b.r }

// SetStep snaps the badge to the rung nearest to v (used to restore a
// persisted step value).
func (b *KnobStepBadge) SetStep(v float64) {
	if len(b.ladder) == 0 {
		return
	}
	best, bestErr := 0, math.Inf(1)
	for i, s := range b.ladder {
		if e := math.Abs(s - v); e < bestErr {
			best, bestErr = i, e
		}
	}
	b.idx = best
}

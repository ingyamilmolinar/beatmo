package ui

import (
	"image"
	"math"
	"strconv"

	"github.com/hajimehoshi/ebiten/v2"
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

// stepLadder builds a strictly-increasing slice of 4 "nice" step sizes for
// def, ranging from a fine rung (~span/2000 notches) to a coarse rung
// (~span/16 notches). Duplicate values are collapsed so the result is always
// strictly increasing.
//
// For a degenerate range (span ≤ 0) a single-element ladder {1} is returned.
func stepLadder(def audio.ParamDef) []float64 {
	span := def.Max - def.Min
	if span <= 0 {
		return []float64{1}
	}
	// divisors control the notch-count at each rung:
	//   span/2000 → very fine (~2000 notches across range)
	//   span/400  → moderate (~400 notches)
	//   span/80   → coarser (~80 notches)
	//   span/16   → coarse  (~16 notches)
	divisors := []float64{2000, 400, 80, 16}
	out := make([]float64, 0, len(divisors))
	for _, d := range divisors {
		s := niceStep(span / d)
		if len(out) == 0 || s > out[len(out)-1] {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		out = []float64{niceStep(span / 400)}
	}
	return out
}

// defaultStepIndex returns the index into ladder whose step produces a
// notch-count closest to 400, giving a nuanced but not overwhelming control.
// The result is always a valid index into ladder.
func defaultStepIndex(def audio.ParamDef, ladder []float64) int {
	span := def.Max - def.Min
	if span <= 0 || len(ladder) == 0 {
		return 0
	}
	best, bestErr := 0, math.Inf(1)
	for i, s := range ladder {
		if e := math.Abs(span/s - 400); e < bestErr {
			best, bestErr = i, e
		}
	}
	return best
}

// formatStepValue formats a step size for display. When def carries a unit
// (e.g. "Hz", "ms", "dB") the result is "100 Hz". For dimensionless params
// the result is "x0.1" — using the ASCII letter x, NOT the multiplication
// glyph, to satisfy the glyph lint on DESIGN.md forbidden-glyph list.
func formatStepValue(def audio.ParamDef, step float64) string {
	s := trimFloat(step)
	if def.Unit == "" {
		return "x" + s
	}
	return s + " " + def.Unit
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
	// wheelAccum counts signed wheel notches toward the next rung change so the
	// mouse wheel feels deliberate and clicky (one rung per
	// knobBadgeWheelNotchesPerRung notches) instead of flying through the
	// short ladder on a single scroll gesture. See WheelResolution.
	wheelAccum int
}

// knobBadgeWheelNotchesPerRung is how many mouse-wheel notches advance the
// resolution badge by ONE rung. >1 makes the wheel less sensitive / clicky.
const knobBadgeWheelNotchesPerRung = 3

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

// Cycle advances to the next rung (wrapping around).
func (b *KnobStepBadge) Cycle() {
	if len(b.ladder) == 0 {
		return
	}
	b.idx = (b.idx + 1) % len(b.ladder)
}

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

// WheelResolution feeds a raw mouse-wheel event toward a rung change. Each
// event counts as a SINGLE notch regardless of its magnitude (so a fast/high-
// resolution wheel can't jump several rungs at once), and a rung only moves
// once knobBadgeWheelNotchesPerRung notches have accumulated in one direction —
// giving a deliberate, clicky feel like the discrete knobs. Positive rawSteps
// = scroll up = finer. Returns true if the rung actually moved.
func (b *KnobStepBadge) WheelResolution(rawSteps int) bool {
	if rawSteps == 0 {
		return false
	}
	dir := 1
	if rawSteps < 0 {
		dir = -1
	}
	// A reversal discards leftover travel so the new direction responds at once
	// rather than first cancelling the old accumulation.
	if b.wheelAccum != 0 && (b.wheelAccum > 0) != (dir > 0) {
		b.wheelAccum = 0
	}
	b.wheelAccum += dir
	if b.wheelAccum > -knobBadgeWheelNotchesPerRung && b.wheelAccum < knobBadgeWheelNotchesPerRung {
		return false
	}
	b.wheelAccum = 0
	return b.AdjustResolution(dir)
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

// Draw renders the pill with the current step label.
func (b *KnobStepBadge) Draw(dst *ebiten.Image) {
	if b.r.Empty() {
		return
	}
	drawRoundedRect(dst, b.r, TokenSurface2(), RadiusXS, true)
	drawRoundedRect(dst, b.r, TokenBorderSubtle(), RadiusXS, false)
	lbl := b.Label()
	tw := TextWidth(lbl)
	tx := b.r.Min.X + (b.r.Dx()-tw)/2
	ty := b.r.Min.Y + (b.r.Dy()-TextHeight())/2
	DrawTextColorAt(dst, lbl, tx, ty, TokenTextSecondary())
}

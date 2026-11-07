package ui

import (
	"image"
	"math"
	"strconv"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// niceStep rounds x UP onto the 1/2/5 × 10^k sequence (… 0.1 0.2 0.5 1 2 5 10 …), the step sizes a
// human counts by. Returns 1 for x ≤ 0.
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

// x must already be a nice step; walking one rung at a time keeps a ladder from skipping one.
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

// The rungs are the consecutive nice steps between ~span/2000 and ~span/16, never a sparse sample, so the
// ladder always offers the round resolutions and never jumps 0.5% to 2%.
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

// Enough detents to sweep the range in a couple of hundred clicks without making one click imperceptible.
const defaultStepNotches = 200

// Starts on the round decade rung (…0.1, 1, 10…) nearest span/defaultStepNotches, so the default step is a
// number a human counts by. Nearness is a ratio (ladders are log-spaced); with no decade rung, the nearest rung.
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

func isDecadeStep(s float64) bool {
	if s <= 0 {
		return false
	}
	l := math.Log10(s)
	return math.Abs(l-math.Round(l)) < 1e-9
}

// The larger over the smaller: 1 means identical. +Inf for non-positive input.
func stepRatioDistance(a, b float64) float64 {
	if a <= 0 || b <= 0 {
		return math.Inf(1)
	}
	if a < b {
		a, b = b, a
	}
	return a / b
}

// "±" marks a step size so the chip never reads as a second value; dimensionless params use ASCII "x0.1"
// because the multiplication glyph is forbidden.
func formatStepValue(def audio.ParamDef, step float64) string {
	s := trimFloat(step)
	if def.Unit == "" {
		return "x" + s
	}
	return "±" + s + " " + def.Unit
}

func trimFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// The chosen rung drives the knob's StepMul and is persisted by param name.
type KnobStepBadge struct {
	def    audio.ParamDef
	ladder []float64
	idx    int
	r      image.Rectangle
}

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

// A positive delta means FINER (lower index: the ladder runs fine to coarse). Clamps, never wraps, so the
// wheel feels bounded.
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

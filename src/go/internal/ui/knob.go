package ui

import (
	"image"
	"math"
)

// KnobScale describes the param domain a Knob represents so the widget can
// snap discrete params to detents, accumulate endless drags in real units,
// and feed the step badge / numeric editor. Set by the owning panel; the
// zero value leaves the knob in legacy continuous-absolute mode.
type KnobScale struct {
	Min, Max float64
	Unit     string   // "", "ms", "Hz", "dB", "%", "st"
	Enum     []string // non-nil => discrete
	Step     float64  // >0 => discrete detents at Min, Min+Step, ... Max
}

// Knob is a HEADLESS value model for a synth/sampler parameter. Since the
// ParamCard + value-wheel-popup redesign (Tasks 3-9) nothing renders or drags
// an in-place rotary dial anymore: the ParamCard draws the value + gauge and
// the wheel popup owns the drag/wheel value interaction. What survives here is
// the pure value math the popup, the wheel nudge helper, the card gauge and the
// EQ code drive: normalized Value in [0,1], the KnobScale domain, and the
// discrete/endless stepping helpers (HandleWheel / StepValue* / NudgeEndless).
type Knob struct {
	r        image.Rectangle
	Value    float64
	dragging bool // headless model never captures; Capturing() is always false

	// Bipolar, when true, renders the value gauge filling outward from the
	// param's neutral point (ZeroFrac) instead of from the track start, so a
	// neutral value (+0 dB gain, 0 cents detune, +0 st pitch) shows no fill
	// and the fill grows on either side of neutral. Read by the ParamCard
	// gauge (see param_card.go); the drag math is unchanged (still a 0..1
	// normalized value).
	Bipolar bool
	// ZeroFrac is the normalized [0,1] position of the param's zero, used
	// only when Bipolar is true. Symmetric ranges (±X) give 0.5; asymmetric
	// ranges (gain -24..+6 dB) give a non-centre value (0.8) so the fill
	// pivots on the true neutral, not the geometric middle.
	ZeroFrac float64

	// Scale describes the param domain (see KnobScale). Discrete/Endless are
	// derived from it by the owning panel and set explicitly.
	Scale KnobScale
	// Discrete makes wheel/step snap to evenly spaced detents (clicky). Set
	// when Scale.Enum != nil || Scale.Step > 0.
	Discrete bool
	// Endless makes stepping accumulate incrementally in real units (StepMul
	// per notch) instead of mapping the rect to 0..1, so the turn never hits a
	// travel wall. Mutually exclusive with Discrete.
	Endless bool
	// StepMul is the real-unit change applied per notch. Set from the step badge.
	StepMul float64
	// valueWheelAccum counts signed wheel notches toward the next value step so
	// a two-finger horizontal trackpad drag moves the value slowly and clicky
	// (one step per knobValueWheelNotchesPerStep notches) instead of racing.
	// See StepValueByWheel.
	valueWheelAccum int
}

const (
	// knobArcSweepDeg is the total angular extent of the value gauge in
	// degrees. 270° leaves a 90° notch at the bottom, mirroring conventional
	// DAW rotaries. Consumed by knobValueArcSpan (the ParamCard bipolar gauge).
	knobArcSweepDeg = 270.0
	// knobArcStartDeg is the angle (in degrees, measured clockwise from the +X
	// axis using screen coords) at which the empty gauge begins.
	knobArcStartDeg = 135.0

	// knobWheelStep is the value delta per wheel notch for legacy continuous
	// knobs (2.5 % of the 0..1 range).
	knobWheelStep = 0.025

	// knobEndlessPxPerNotch is the horizontal pixels that equal one StepMul of
	// value change in endless mode. Used by the wheel-nudge / popup callers.
	knobEndlessPxPerNotch = 8
)

// knobValueArcSpan returns the [from,to] angular span (radians) the filled
// value gauge should occupy for a normalized value in [0,1], plus whether there
// is anything to draw. Pure function of value + polarity so it is unit-tested
// directly (knob_bipolar_test.go).
//
//   - Unipolar: the arc runs from the track start to the value angle; value 0
//     draws nothing.
//   - Bipolar: the arc runs between the neutral angle (zeroFrac of the sweep)
//     and the value angle (neutral→value above zero, value→neutral below), so
//     value == zeroFrac draws nothing and the fill grows on either side of the
//     param's true zero.
func knobValueArcSpan(value, zeroFrac float64, bipolar bool) (from, to float32, draw bool) {
	start := float32(knobArcStartDeg * math.Pi / 180)
	valRad := start + float32(value*knobArcSweepDeg*math.Pi/180)
	if !bipolar {
		if valRad > start {
			return start, valRad, true
		}
		return 0, 0, false
	}
	zeroRad := start + float32(zeroFrac*knobArcSweepDeg*math.Pi/180)
	switch {
	case valRad > zeroRad:
		return zeroRad, valRad, true
	case valRad < zeroRad:
		return valRad, zeroRad, true
	default:
		return 0, 0, false
	}
}

// NewKnob returns a Knob initialised to v (clamped to [0,1]).
func NewKnob(v float64) *Knob {
	if v < 0 {
		v = 0
	} else if v > 1 {
		v = 1
	}
	return &Knob{Value: v}
}

// SetRect sets the bounding rectangle the knob's ParamCard occupies. The owning
// panel derives value-wheel-popup anchors and hit areas from it.
func (k *Knob) SetRect(r image.Rectangle) { k.r = r }

// Rect returns the current bounding rectangle.
func (k *Knob) Rect() image.Rectangle { return k.r }

// Capturing reports whether a drag is in progress. The headless model never
// captures a drag (the value-wheel popup owns the drag), so this is always
// false; it is retained because sampler/synth layout guards still call it.
func (k *Knob) Capturing() bool { return k.dragging }

// HandleWheel adjusts the value by `steps` wheel notches. For discrete knobs,
// each notch advances exactly one detent index. For endless knobs, each notch
// applies ±StepMul real units (clamped to [Min,Max]). For legacy continuous
// knobs the original knobWheelStep behaviour is preserved. Returns
// InputConsumed when the value changed, InputIgnored when the cursor is
// outside the knob's rect or the value is already at its limit.
func (k *Knob) HandleWheel(mx, my, steps int) InputResult {
	if !image.Pt(mx, my).In(k.r) {
		return InputIgnored
	}
	if k.Discrete {
		n := k.detentCount()
		if n >= 2 {
			idx := int(math.Round(k.Value*float64(n-1))) + steps
			if idx < 0 {
				idx = 0
			} else if idx > n-1 {
				idx = n - 1
			}
			nv := float64(idx) / float64(n-1)
			if nv == k.Value {
				return InputIgnored
			}
			k.Value = nv
			return InputConsumed
		}
	}
	if k.Endless {
		span := k.Scale.Max - k.Scale.Min
		if span > 0 && k.StepMul > 0 {
			real := clampF64(k.Scale.Min+k.Value*span+float64(steps)*k.StepMul, k.Scale.Min, k.Scale.Max)
			nv := (real - k.Scale.Min) / span
			if nv == k.Value {
				return InputIgnored
			}
			k.Value = nv
			return InputConsumed
		}
	}
	v := k.Value + float64(steps)*knobWheelStep
	if v < 0 {
		v = 0
	} else if v > 1 {
		v = 1
	}
	if v == k.Value {
		return InputIgnored
	}
	k.Value = v
	return InputConsumed
}

// knobValueWheelNotchesPerStep is how many two-finger/wheel notches advance the
// knob VALUE by one step. >1 makes the wheel slow and human-paced; raise it to
// slow further. (The per-step AMOUNT is StepMul, which the user controls via
// the resolution badge — so resolution × cadence both shape the feel.)
const knobValueWheelNotchesPerStep = 4

// StepValueByWheel feeds a raw wheel/two-finger delta toward a value change.
// Each call counts as a SINGLE notch regardless of magnitude (so a fast or
// high-resolution trackpad can't race the value), and the value only advances
// by ONE step once knobValueWheelNotchesPerStep notches accumulate in one
// direction — giving a deliberate, easy-to-control feel. One step = ±StepMul
// for endless knobs (honoring the user's chosen resolution) or ±1 detent for
// discrete knobs. A reversal discards leftover travel. Returns true if the
// value changed. Unlike HandleWheel it does NOT range-check the cursor, so it
// works whether the gesture is over the dial, the caption, or the readout.
func (k *Knob) StepValueByWheel(steps int) bool {
	if steps == 0 {
		return false
	}
	dir := 1
	if steps < 0 {
		dir = -1
	}
	if k.valueWheelAccum != 0 && (k.valueWheelAccum > 0) != (dir > 0) {
		k.valueWheelAccum = 0
	}
	k.valueWheelAccum += dir
	if k.valueWheelAccum > -knobValueWheelNotchesPerStep && k.valueWheelAccum < knobValueWheelNotchesPerStep {
		return false
	}
	k.valueWheelAccum = 0
	return k.applyValueStep(dir)
}

// StepValueDirect advances the value by exactly ONE step in dir (+1/-1) with
// no internal accumulator — one detent for discrete knobs, one StepMul for
// endless knobs. Use this when the caller already manages its own debounce
// accumulator (e.g. MobileWheelPopup's discAcc), so the extra per-notch
// threshold of StepValueByWheel is not desirable.
func (k *Knob) StepValueDirect(dir int) bool { return k.applyValueStep(dir) }

// applyValueStep moves the value by exactly one step in dir (+1/-1) — one
// detent for discrete knobs, one StepMul for endless knobs — with no cursor
// range check. Returns true if the value changed.
func (k *Knob) applyValueStep(dir int) bool {
	if k.Discrete {
		n := k.detentCount()
		if n < 2 {
			return false
		}
		idx := int(math.Round(k.Value*float64(n-1))) + dir
		if idx < 0 {
			idx = 0
		} else if idx > n-1 {
			idx = n - 1
		}
		nv := float64(idx) / float64(n-1)
		if nv == k.Value {
			return false
		}
		k.Value = nv
		return true
	}
	span := k.Scale.Max - k.Scale.Min
	if span <= 0 {
		return false
	}
	step := k.StepMul
	if step <= 0 {
		step = span / 200
	}
	real := clampF64(k.Scale.Min+k.Value*span+float64(dir)*step, k.Scale.Min, k.Scale.Max)
	nv := (real - k.Scale.Min) / span
	if nv == k.Value {
		return false
	}
	k.Value = nv
	return true
}

// detentCount returns the number of discrete positions, or 0 if continuous.
func (k *Knob) detentCount() int {
	if len(k.Scale.Enum) > 0 {
		return len(k.Scale.Enum)
	}
	if k.Scale.Step > 0 && k.Scale.Max > k.Scale.Min {
		return int(math.Round((k.Scale.Max-k.Scale.Min)/k.Scale.Step)) + 1
	}
	return 0
}

// NudgeEndless accumulates an endless value change by deltaPx pixels of motion
// (positive = increase). The caller owns the per-frame delta, so this carries
// no latched state and is safe to drive from the mobile wheel popup's vertical
// drag and the desktop wheel-nudge helper. Clamps at Scale.Min/Max.
func (k *Knob) NudgeEndless(deltaPx int) {
	span := k.Scale.Max - k.Scale.Min
	if span <= 0 {
		return
	}
	step := k.StepMul
	if step <= 0 {
		step = span / 200
	}
	real := k.Scale.Min + k.Value*span
	real = clampF64(real+(float64(deltaPx)/float64(knobEndlessPxPerNotch))*step, k.Scale.Min, k.Scale.Max)
	k.Value = (real - k.Scale.Min) / span
}

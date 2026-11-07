package ui

import (
	"image"
	"math"
)

// The zero value leaves the knob in continuous-absolute mode.
type KnobScale struct {
	Min, Max float64
	Unit     string   // "", "ms", "Hz", "dB", "%", "st"
	Enum     []string // non-nil => discrete
	Step     float64  // >0 => discrete detents at Min, Min+Step, ... Max
}

// Knob is a headless value model: the ParamCard draws it and the wheel popup owns the drag; only the value
// math (normalized Value, the KnobScale domain, discrete/endless stepping) lives here.
type Knob struct {
	r        image.Rectangle
	Value    float64
	dragging bool // headless model never captures; Capturing() is always false

	// Bipolar gauges fill outward from ZeroFrac, so a neutral value (+0 dB, 0 cents) shows no fill.
	Bipolar bool
	// ZeroFrac is the normalized position of the param's zero: 0.5 for ±X, 0.8 for -24..+6 dB, so the fill
	// pivots on the true neutral, not the geometric middle.
	ZeroFrac float64

	Scale    KnobScale
	Discrete bool
	// Endless steps accumulate in real units (StepMul per notch) so a turn never hits a travel wall.
	// Mutually exclusive with Discrete.
	Endless bool
	// StepMul is the real-unit change applied per notch. Set from the step badge.
	StepMul float64
	// Counts wheel notches toward the next step so a trackpad drag moves the value slowly instead of racing.
	valueWheelAccum int
}

const (
	// 270° leaves the conventional 90° notch at the bottom.
	knobArcSweepDeg = 270.0
	// Degrees clockwise from +X in screen coordinates.
	knobArcStartDeg = 135.0

	knobWheelStep = 0.025

	knobEndlessPxPerNotch = 8
)

// Bipolar arcs run between the neutral angle and the value angle, so value == zeroFrac draws nothing;
// unipolar arcs start at the track start.
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

func (k *Knob) SetRect(r image.Rectangle) { k.r = r }

// Rect returns the current bounding rectangle.
func (k *Knob) Rect() image.Rectangle { return k.r }

// Always false (the popup owns the drag); kept because the sampler/synth layout guards still call it.
func (k *Knob) Capturing() bool { return k.dragging }

// Discrete: one detent per notch; endless: ±StepMul clamped to [Min,Max]. Ignored outside the rect or at a limit.
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

// Notches per value step; >1 keeps the wheel human-paced. The step AMOUNT is StepMul, set by the badge.
const knobValueWheelNotchesPerStep = 4

// Each call is ONE notch whatever its magnitude, so a fast trackpad cannot race the value; a reversal
// discards leftover travel. No cursor check, so the gesture works over any part of the card.
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

// No accumulator: for callers that debounce themselves (MobileWheelPopup).
func (k *Knob) StepValueDirect(dir int) bool { return k.applyValueStep(dir) }

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

// The caller owns the per-frame delta, so this carries no latched state. Clamps at Scale.Min/Max.
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

//go:build test || js

package audio

import "math"

// clampf constrains v to [lo, hi].
func clampf(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// distortion implements a tanh soft-clip waveshaper with a one-pole lowpass
// tone filter and wet/dry mix. Higher drive values produce more saturation.
type distortion struct {
	drive float64 // 1-20: gain before tanh
	tone  float64 // LP filter cutoff Hz
	mix   float64 // 0=dry, 1=full wet

	sr   int
	lpY1 float64  // one-pole LP state
	lpA  float64  // one-pole coefficient
}

func newDistortion(sr int, params map[string]float64) *distortion {
	d := &distortion{sr: sr}
	d.drive = clampf(params["drive"], 1, 20)
	d.tone = clampf(params["tone"], 200, 8000)
	d.mix = clampf(params["mix"], 0, 1)
	d.recalcLP()
	return d
}

func (d *distortion) recalcLP() {
	// One-pole lowpass: a = exp(-2π * fc / sr)
	fc := d.tone
	if fc <= 0 {
		fc = 4000
	}
	sr := float64(d.sr)
	if sr <= 0 {
		sr = 44100
	}
	d.lpA = math.Exp(-2.0 * math.Pi * fc / sr)
}

func (d *distortion) ProcessSample(x float64) float64 {
	// Waveshape: tanh(drive * x)
	wet := math.Tanh(d.drive * x)
	// Tone filter: one-pole LP on the wet signal
	d.lpY1 = wet*(1-d.lpA) + d.lpY1*d.lpA
	wet = d.lpY1
	// Mix
	return x*(1-d.mix) + wet*d.mix
}

func (d *distortion) Reset() {
	d.lpY1 = 0
}

func (d *distortion) SetParam(name string, value float64) {
	switch name {
	case "drive":
		d.drive = clampf(value, 1, 20)
	case "tone":
		d.tone = clampf(value, 200, 8000)
		d.recalcLP()
	case "mix":
		d.mix = clampf(value, 0, 1)
	}
}

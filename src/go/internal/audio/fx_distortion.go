//go:build test || js

package audio

import "math"

// distortion implements a tanh soft-clip waveshaper with a one-pole lowpass
// tone filter and wet/dry mix. Higher drive values produce more saturation.
// All user-facing parameters are smoothed to prevent clicks on change.
type distortion struct {
	drive smoothParam // 1-20: gain before tanh
	tone  smoothParam // LP filter cutoff Hz
	mix   smoothParam // 0=dry, 1=full wet

	sr   int
	lpY1 float64 // one-pole LP state
	lpA  float64 // one-pole coefficient
}

func newDistortion(sr int, params map[string]float64) *distortion {
	d := &distortion{sr: sr}
	d.drive = newSmoothParam(clampf(params["drive"], 1, 20), sr, defaultSmoothTimeMs)
	d.tone = newSmoothParam(clampf(params["tone"], 200, 8000), sr, defaultSmoothTimeMs)
	d.mix = newSmoothParam(clampf(params["mix"], 0, 1), sr, defaultSmoothTimeMs)
	d.recalcLP()
	return d
}

func (d *distortion) recalcLP() {
	// One-pole lowpass: a = exp(-2π * fc / sr)
	fc := d.tone.value()
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
	drv := d.drive.tick()
	mix := d.mix.tick()

	// Smoothly update tone filter coefficient.
	toneVal := d.tone.tick()
	sr := float64(d.sr)
	if sr <= 0 {
		sr = 44100
	}
	d.lpA = math.Exp(-2.0 * math.Pi * toneVal / sr)

	// Waveshape: tanh(drive * x)
	wet := math.Tanh(drv * x)
	// Tone filter: one-pole LP on the wet signal
	d.lpY1 = wet*(1-d.lpA) + d.lpY1*d.lpA
	wet = d.lpY1
	// Mix
	return x*(1-mix) + wet*mix
}

func (d *distortion) Reset() {
	d.lpY1 = 0
}

func (d *distortion) SetParam(name string, value float64) {
	switch name {
	case "drive":
		d.drive.set(clampf(value, 1, 20))
	case "tone":
		d.tone.set(clampf(value, 200, 8000))
	case "mix":
		d.mix.set(clampf(value, 0, 1))
	}
}

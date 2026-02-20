//go:build test || js

package audio

import "math"

// ringMod implements a ring modulator effect. It multiplies the input signal
// by an internal carrier oscillator (sine or square wave), producing sum and
// difference frequency components that create metallic, bell-like timbres.
type ringMod struct {
	frequency float64 // carrier frequency Hz (20-5000)
	shape     float64 // 0=sine, 1=square
	mix       float64 // wet/dry 0-1

	sr       int
	phase    float64 // oscillator phase 0..2π
	phaseInc float64 // per-sample phase increment
}

func newRingMod(sr int, params map[string]float64) *ringMod {
	r := &ringMod{sr: sr}
	r.frequency = clampf(params["frequency"], 20, 5000)
	r.shape = clampf(params["shape"], 0, 1)
	r.mix = clampf(params["mix"], 0, 1)
	r.recalc()
	return r
}

func (r *ringMod) recalc() {
	sr := float64(r.sr)
	if sr <= 0 {
		sr = 44100
	}
	r.phaseInc = 2.0 * math.Pi * r.frequency / sr
}

func (r *ringMod) ProcessSample(x float64) float64 {
	// Generate carrier
	var carrier float64
	if r.shape < 0.5 {
		carrier = math.Sin(r.phase)
	} else {
		// Square wave: +1 for first half, -1 for second half
		if r.phase < math.Pi {
			carrier = 1.0
		} else {
			carrier = -1.0
		}
	}

	wet := x * carrier

	// Advance phase
	r.phase += r.phaseInc
	if r.phase >= 2*math.Pi {
		r.phase -= 2 * math.Pi
	}

	return x*(1-r.mix) + wet*r.mix
}

func (r *ringMod) Reset() {
	r.phase = 0
}

func (r *ringMod) SetParam(name string, value float64) {
	switch name {
	case "frequency":
		r.frequency = clampf(value, 20, 5000)
		r.recalc()
	case "shape":
		r.shape = clampf(value, 0, 1)
	case "mix":
		r.mix = clampf(value, 0, 1)
	}
}

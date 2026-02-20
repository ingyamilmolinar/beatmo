//go:build test || js

package audio

import "math"

const (
	autowahBaseFreq = 200.0  // Hz — minimum filter cutoff
	autowahMaxFreq  = 4000.0 // Hz — maximum filter cutoff
	autowahFilterQ  = 2.0    // resonance factor
)

// autoWah implements an envelope-following auto-wah (dynamic bandpass filter).
// The filter cutoff is modulated by a combination of the input signal's
// amplitude envelope and an internal LFO, producing a wah-wah sweep.
type autoWah struct {
	sensitivity float64 // envelope sensitivity 0-1
	rate        float64 // LFO rate Hz
	depth       float64 // LFO depth 0-1
	mix         float64 // wet/dry 0-1

	sr          int
	envFollower float64 // envelope follower state
	lfoPhase    float64 // LFO phase 0..2π
	lfoPhaseInc float64 // per-sample LFO phase increment

	// State-variable filter state
	bp float64 // bandpass output
	lp float64 // lowpass output
}

func newAutoWah(sr int, params map[string]float64) *autoWah {
	a := &autoWah{sr: sr}
	a.sensitivity = clampf(params["sensitivity"], 0, 1)
	a.rate = clampf(params["rate"], 0.5, 20)
	a.depth = clampf(params["depth"], 0, 1)
	a.mix = clampf(params["mix"], 0, 1)
	a.recalc()
	return a
}

func (a *autoWah) recalc() {
	sr := float64(a.sr)
	if sr <= 0 {
		sr = 44100
	}
	a.lfoPhaseInc = 2.0 * math.Pi * a.rate / sr
}

func (a *autoWah) ProcessSample(x float64) float64 {
	sr := float64(a.sr)
	if sr <= 0 {
		sr = 44100
	}

	// Envelope follower (fast attack, slow release)
	absX := math.Abs(x)
	const attackCoef = 0.001
	const releaseCoef = 0.9995
	if absX > a.envFollower {
		a.envFollower = attackCoef*a.envFollower + (1-attackCoef)*absX
	} else {
		a.envFollower = releaseCoef * a.envFollower
	}

	// LFO modulation (0..1 range)
	lfo := (math.Sin(a.lfoPhase) + 1) * 0.5
	a.lfoPhase += a.lfoPhaseInc
	if a.lfoPhase >= 2*math.Pi {
		a.lfoPhase -= 2 * math.Pi
	}

	// Compute cutoff from envelope + LFO
	envMod := a.envFollower * a.sensitivity * 10.0
	if envMod > 1.0 {
		envMod = 1.0
	}
	lfoMod := lfo * a.depth
	modTotal := clampf(envMod+lfoMod, 0, 1)
	cutoff := autowahBaseFreq + modTotal*(autowahMaxFreq-autowahBaseFreq)

	// State-variable filter (bandpass)
	f := 2.0 * math.Sin(math.Pi*cutoff/sr)
	if f > 0.99 {
		f = 0.99
	}
	q := 1.0 / autowahFilterQ

	hp := x - a.lp - q*a.bp
	a.bp += f * hp
	a.lp += f * a.bp
	wet := a.bp // bandpass output

	return x*(1-a.mix) + wet*a.mix
}

func (a *autoWah) Reset() {
	a.envFollower = 0
	a.lfoPhase = 0
	a.bp = 0
	a.lp = 0
}

func (a *autoWah) SetParam(name string, value float64) {
	switch name {
	case "sensitivity":
		a.sensitivity = clampf(value, 0, 1)
	case "rate":
		a.rate = clampf(value, 0.5, 20)
		a.recalc()
	case "depth":
		a.depth = clampf(value, 0, 1)
	case "mix":
		a.mix = clampf(value, 0, 1)
	}
}

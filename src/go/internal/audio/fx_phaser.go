//go:build test || js

package audio

import "math"

// phaser implements a chain of first-order allpass filters with an LFO sweeping
// the cutoff frequency. The number of stages controls the depth of the notches
// in the frequency response, while feedback reinforces the effect.
type phaser struct {
	stages   int     // number of allpass stages (2-12)
	rate     float64 // LFO frequency Hz
	depth    float64 // sweep depth 0-1
	feedback float64 // feedback amount 0-0.95
	mix      float64 // wet/dry 0-1

	sr       int
	phase    float64    // LFO phase 0..2π
	x1       [12]float64 // allpass input state per stage
	y1       [12]float64 // allpass output state per stage
	fbSample float64    // feedback sample from last stage
}

func newPhaser(sr int, params map[string]float64) *phaser {
	p := &phaser{sr: sr}
	p.stages = int(clampf(params["stages"], 2, 12))
	p.rate = clampf(params["rate"], 0.1, 10)
	p.depth = clampf(params["depth"], 0, 1)
	p.feedback = clampf(params["feedback"], 0, 0.95)
	p.mix = clampf(params["mix"], 0, 1)
	return p
}

func (p *phaser) ProcessSample(x float64) float64 {
	sr := float64(p.sr)
	if sr <= 0 {
		sr = 44100
	}

	// Add feedback from previous iteration
	in := x + p.fbSample*p.feedback

	// LFO: sine wave mapped to 0..1
	lfo := (math.Sin(p.phase) + 1) * 0.5

	// Sweep frequency range: 200 Hz to 200 + depth*3800 Hz
	minFreq := 200.0
	maxFreq := 200.0 + p.depth*3800.0
	fc := minFreq + lfo*(maxFreq-minFreq)

	// Allpass coefficient: w = tan(pi * fc / sr), a = (w-1)/(w+1)
	w := math.Tan(math.Pi * fc / sr)
	a := (w - 1) / (w + 1)

	// Process through each allpass stage
	for s := 0; s < p.stages; s++ {
		y := a*in + p.x1[s] - a*p.y1[s]
		p.x1[s] = in
		p.y1[s] = y
		in = y
	}

	// Store feedback sample (output of last stage)
	p.fbSample = in

	// Advance LFO phase
	p.phase += 2 * math.Pi * p.rate / sr
	if p.phase >= 2*math.Pi {
		p.phase -= 2 * math.Pi
	}

	// Mix: dry*(1-mix) + wet*mix
	return x*(1-p.mix) + in*p.mix
}

func (p *phaser) Reset() {
	p.phase = 0
	p.fbSample = 0
	for i := range p.x1 {
		p.x1[i] = 0
	}
	for i := range p.y1 {
		p.y1[i] = 0
	}
}

func (p *phaser) SetParam(name string, value float64) {
	switch name {
	case "stages":
		p.stages = int(clampf(value, 2, 12))
	case "rate":
		p.rate = clampf(value, 0.1, 10)
	case "depth":
		p.depth = clampf(value, 0, 1)
	case "feedback":
		p.feedback = clampf(value, 0, 0.95)
	case "mix":
		p.mix = clampf(value, 0, 1)
	}
}

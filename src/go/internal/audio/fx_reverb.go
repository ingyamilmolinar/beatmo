//go:build test || js

package audio

import "math"

// reverb implements a Schroeder reverb: 4 parallel comb filters feeding
// 2 series allpass filters, with damping and room size control.
type reverb struct {
	room    float64 // 0-1: scales comb feedback
	damping float64 // 0-1: LP damping in combs
	mix     float64 // wet/dry 0-1

	sr    int
	combs [4]combFilter
	aps   [2]allpassFilter
}

// Comb delay lengths in samples at 44100 Hz (Schroeder classic, scaled for sr).
var combDelays = [4]int{1116, 1188, 1277, 1356}
var apDelays = [2]int{556, 441}

func newReverb(sr int, params map[string]float64) *reverb {
	r := &reverb{sr: sr}
	r.room = clampf(params["room"], 0, 1)
	r.damping = clampf(params["damping"], 0, 1)
	r.mix = clampf(params["mix"], 0, 1)
	r.initFilters()
	return r
}

func (r *reverb) initFilters() {
	scale := float64(r.sr) / 44100.0
	if scale <= 0 {
		scale = 1
	}
	for i := 0; i < 4; i++ {
		length := int(math.Round(float64(combDelays[i]) * scale))
		if length < 1 {
			length = 1
		}
		r.combs[i] = combFilter{
			buf:      make([]float64, length),
			feedback: 0.7 + 0.28*r.room, // 0.7 - 0.98 range
			damping:  r.damping,
		}
	}
	for i := 0; i < 2; i++ {
		length := int(math.Round(float64(apDelays[i]) * scale))
		if length < 1 {
			length = 1
		}
		r.aps[i] = allpassFilter{
			buf: make([]float64, length),
			g:   0.5,
		}
	}
}

func (r *reverb) ProcessSample(x float64) float64 {
	// Sum 4 parallel comb filters
	var wet float64
	for i := range r.combs {
		wet += r.combs[i].process(x)
	}
	wet *= 0.25 // normalize

	// Series allpass filters
	for i := range r.aps {
		wet = r.aps[i].process(wet)
	}

	return x*(1-r.mix) + wet*r.mix
}

func (r *reverb) Reset() {
	for i := range r.combs {
		r.combs[i].reset()
	}
	for i := range r.aps {
		r.aps[i].reset()
	}
}

func (r *reverb) SetParam(name string, value float64) {
	switch name {
	case "room":
		r.room = clampf(value, 0, 1)
		for i := range r.combs {
			r.combs[i].feedback = 0.7 + 0.28*r.room
		}
	case "damping":
		r.damping = clampf(value, 0, 1)
		for i := range r.combs {
			r.combs[i].damping = r.damping
		}
	case "mix":
		r.mix = clampf(value, 0, 1)
	}
}

// combFilter is a feedback comb filter with one-pole LP damping.
type combFilter struct {
	buf      []float64
	pos      int
	feedback float64
	damping  float64
	lpState  float64 // one-pole LP state
}

func (c *combFilter) process(x float64) float64 {
	bufLen := len(c.buf)
	if bufLen == 0 {
		return x
	}
	out := c.buf[c.pos]

	// LP damping on feedback: y = out*(1-d) + lpState*d
	c.lpState = out*(1-c.damping) + c.lpState*c.damping
	c.buf[c.pos] = x + c.lpState*c.feedback

	c.pos++
	if c.pos >= bufLen {
		c.pos = 0
	}
	return out
}

func (c *combFilter) reset() {
	for i := range c.buf {
		c.buf[i] = 0
	}
	c.pos = 0
	c.lpState = 0
}

// allpassFilter is a Schroeder allpass with coefficient g.
type allpassFilter struct {
	buf []float64
	pos int
	g   float64
}

func (a *allpassFilter) process(x float64) float64 {
	bufLen := len(a.buf)
	if bufLen == 0 {
		return x
	}
	delayed := a.buf[a.pos]
	a.buf[a.pos] = x + delayed*a.g
	a.pos++
	if a.pos >= bufLen {
		a.pos = 0
	}
	return delayed - x*a.g
}

func (a *allpassFilter) reset() {
	for i := range a.buf {
		a.buf[i] = 0
	}
	a.pos = 0
}

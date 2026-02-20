//go:build test || js

package audio

import "math"

// chorus implements an LFO-modulated interpolated delay line with wet/dry mix.
// The LFO modulates the read position of a short delay to create pitch variation.
type chorus struct {
	rate  float64 // LFO frequency Hz
	depth float64 // modulation depth in ms
	mix   float64 // wet/dry 0-1

	sr    int
	buf   []float64 // delay buffer
	pos   int
	phase float64 // LFO phase 0..2π

	// Derived
	depthSamples float64 // depth in samples
	phaseInc     float64 // per-sample phase increment
}

func newChorus(sr int, params map[string]float64) *chorus {
	c := &chorus{sr: sr}
	c.rate = clampf(params["rate"], 0.1, 10)
	c.depth = clampf(params["depth"], 0, 20)
	c.mix = clampf(params["mix"], 0, 1)
	c.recalc()
	return c
}

func (c *chorus) recalc() {
	sr := float64(c.sr)
	if sr <= 0 {
		sr = 44100
	}
	c.depthSamples = c.depth * 0.001 * sr
	c.phaseInc = 2.0 * math.Pi * c.rate / sr

	// Buffer needs to hold max depth + some margin
	needed := int(c.depthSamples*2) + 4
	if needed < 64 {
		needed = 64
	}
	if len(c.buf) < needed {
		c.buf = make([]float64, needed)
		c.pos = 0
	}
}

func (c *chorus) ProcessSample(x float64) float64 {
	bufLen := len(c.buf)
	if bufLen == 0 {
		return x
	}

	// Write input
	c.buf[c.pos] = x

	// LFO: sine 0..1 range, centered modulation
	lfo := (math.Sin(c.phase) + 1) * 0.5 // 0..1
	delaySamples := 1.0 + lfo*c.depthSamples

	// Read with linear interpolation
	readF := float64(c.pos) - delaySamples
	if readF < 0 {
		readF += float64(bufLen)
	}
	idx0 := int(readF) % bufLen
	idx1 := (idx0 + 1) % bufLen
	frac := readF - math.Floor(readF)
	wet := c.buf[idx0]*(1-frac) + c.buf[idx1]*frac

	// Advance LFO
	c.phase += c.phaseInc
	if c.phase >= 2*math.Pi {
		c.phase -= 2 * math.Pi
	}

	// Advance write position
	c.pos++
	if c.pos >= bufLen {
		c.pos = 0
	}

	return x*(1-c.mix) + wet*c.mix
}

func (c *chorus) Reset() {
	for i := range c.buf {
		c.buf[i] = 0
	}
	c.pos = 0
	c.phase = 0
}

func (c *chorus) SetParam(name string, value float64) {
	switch name {
	case "rate":
		c.rate = clampf(value, 0.1, 10)
		c.recalc()
	case "depth":
		c.depth = clampf(value, 0, 20)
		c.recalc()
	case "mix":
		c.mix = clampf(value, 0, 1)
	}
}

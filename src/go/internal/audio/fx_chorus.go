//go:build test || js

package audio

import "math"

// chorus implements an LFO-modulated interpolated delay line with wet/dry mix.
// The LFO modulates the read position of a short delay to create pitch variation.
// Parameters are smoothed to prevent clicks. Buffer is pre-allocated to max depth
// to avoid reallocation pops.
type chorus struct {
	rate  smoothParam // LFO frequency Hz
	depth smoothParam // modulation depth in ms
	mix   smoothParam // wet/dry 0-1

	sr    int
	buf   []float64 // delay buffer (pre-allocated to max depth)
	pos   int
	phase float64 // LFO phase 0..2π
}

const chorusMaxDepthMs = 20.0

func newChorus(sr int, params map[string]float64) *chorus {
	c := &chorus{sr: sr}
	c.rate = newSmoothParam(clampf(params["rate"], 0.1, 10), sr, defaultSmoothTimeMs)
	c.depth = newSmoothParam(clampf(params["depth"], 0, chorusMaxDepthMs), sr, defaultSmoothTimeMs)
	c.mix = newSmoothParam(clampf(params["mix"], 0, 1), sr, defaultSmoothTimeMs)

	// Pre-allocate buffer for max depth to avoid reallocation on depth change.
	allocSR := float64(sr)
	if allocSR <= 0 {
		allocSR = 44100
	}
	maxSamples := int(chorusMaxDepthMs*0.001*allocSR*2) + 4
	if maxSamples < 64 {
		maxSamples = 64
	}
	c.buf = make([]float64, maxSamples)
	return c
}

func (c *chorus) ProcessSample(x float64) float64 {
	bufLen := len(c.buf)
	if bufLen == 0 {
		return x
	}

	rate := c.rate.tick()
	depthMs := c.depth.tick()
	mix := c.mix.tick()

	sr := float64(c.sr)
	if sr <= 0 {
		sr = 44100
	}
	depthSamples := depthMs * 0.001 * sr
	phaseInc := 2.0 * math.Pi * rate / sr

	// Write input
	c.buf[c.pos] = x

	// LFO: sine 0..1 range, centered modulation
	lfo := (math.Sin(c.phase) + 1) * 0.5 // 0..1
	delaySamples := 1.0 + lfo*depthSamples

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
	c.phase += phaseInc
	if c.phase >= 2*math.Pi {
		c.phase -= 2 * math.Pi
	}

	// Advance write position
	c.pos++
	if c.pos >= bufLen {
		c.pos = 0
	}

	return x*(1-mix) + wet*mix
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
		c.rate.set(clampf(value, 0.1, 10))
	case "depth":
		c.depth.set(clampf(value, 0, chorusMaxDepthMs))
	case "mix":
		c.mix.set(clampf(value, 0, 1))
	}
}

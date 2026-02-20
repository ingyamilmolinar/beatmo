//go:build test || js

package audio

import "math"

// flanger implements a short modulated delay line with signed feedback.
// Similar to chorus but with shorter delay times and feedback, producing
// the characteristic jet-sweep sound through comb filtering.
type flanger struct {
	rate     float64 // LFO frequency Hz
	depth    float64 // modulation depth in ms (0.5-10)
	feedback float64 // feedback amount -0.95 to 0.95
	mix      float64 // wet/dry 0-1

	sr       int
	buf      []float64 // circular delay buffer
	pos      int
	phase    float64 // LFO phase 0..2π
	lastRead float64 // last read value for feedback

	// Derived
	depthSamples float64 // depth in samples
	phaseInc     float64 // per-sample phase increment
}

func newFlanger(sr int, params map[string]float64) *flanger {
	f := &flanger{sr: sr}
	f.rate = clampf(params["rate"], 0.1, 10)
	f.depth = clampf(params["depth"], 0.5, 10)
	f.feedback = clampf(params["feedback"], -0.95, 0.95)
	f.mix = clampf(params["mix"], 0, 1)
	f.recalc()
	return f
}

func (f *flanger) recalc() {
	sr := float64(f.sr)
	if sr <= 0 {
		sr = 44100
	}
	f.depthSamples = f.depth * 0.001 * sr
	f.phaseInc = 2.0 * math.Pi * f.rate / sr

	// Buffer needs to hold max depth (10ms) + margin
	needed := int(f.depthSamples*2) + 4
	if needed < 64 {
		needed = 64
	}
	if len(f.buf) < needed {
		f.buf = make([]float64, needed)
		f.pos = 0
	}
}

func (f *flanger) ProcessSample(x float64) float64 {
	bufLen := len(f.buf)
	if bufLen == 0 {
		return x
	}

	// Write input + feedback to buffer
	f.buf[f.pos] = x + f.feedback*f.lastRead

	// LFO: sine 0..1 range
	lfo := (math.Sin(f.phase) + 1) * 0.5
	delaySamples := 1.0 + lfo*f.depthSamples

	// Read with linear interpolation
	readF := float64(f.pos) - delaySamples
	if readF < 0 {
		readF += float64(bufLen)
	}
	idx0 := int(readF) % bufLen
	idx1 := (idx0 + 1) % bufLen
	frac := readF - math.Floor(readF)
	wet := f.buf[idx0]*(1-frac) + f.buf[idx1]*frac

	// Store for next feedback iteration
	f.lastRead = wet

	// Advance LFO
	f.phase += f.phaseInc
	if f.phase >= 2*math.Pi {
		f.phase -= 2 * math.Pi
	}

	// Advance write position
	f.pos++
	if f.pos >= bufLen {
		f.pos = 0
	}

	return x*(1-f.mix) + wet*f.mix
}

func (f *flanger) Reset() {
	for i := range f.buf {
		f.buf[i] = 0
	}
	f.pos = 0
	f.phase = 0
	f.lastRead = 0
}

func (f *flanger) SetParam(name string, value float64) {
	switch name {
	case "rate":
		f.rate = clampf(value, 0.1, 10)
		f.recalc()
	case "depth":
		f.depth = clampf(value, 0.5, 10)
		f.recalc()
	case "feedback":
		f.feedback = clampf(value, -0.95, 0.95)
	case "mix":
		f.mix = clampf(value, 0, 1)
	}
}

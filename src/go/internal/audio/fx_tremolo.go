//go:build test || js

package audio

import "math"

// tremolo implements LFO-based amplitude modulation. The LFO shape can be
// sine, triangle, or square wave, and modulates the signal's gain.
type tremolo struct {
	rate  float64 // LFO frequency Hz (0.5-20)
	depth float64 // modulation depth 0-1
	shape int     // 0=sine, 1=triangle, 2=square
	mix   float64 // wet/dry 0-1

	sr    int
	phase float64 // LFO phase 0..2π
}

func newTremolo(sr int, params map[string]float64) *tremolo {
	t := &tremolo{sr: sr}
	t.rate = clampf(params["rate"], 0.5, 20)
	t.depth = clampf(params["depth"], 0, 1)
	t.shape = int(clampf(params["shape"], 0, 2))
	t.mix = clampf(params["mix"], 0, 1)
	return t
}

func (t *tremolo) lfoValue() float64 {
	switch t.shape {
	case 1: // triangle: abs(phase/pi - 1), maps 0..2pi to 0..1..0
		return math.Abs(t.phase/math.Pi - 1)
	case 2: // square
		if t.phase < math.Pi {
			return 1.0
		}
		return 0.0
	default: // sine
		return (math.Sin(t.phase) + 1) * 0.5
	}
}

func (t *tremolo) ProcessSample(x float64) float64 {
	sr := float64(t.sr)
	if sr <= 0 {
		sr = 44100
	}

	lfo := t.lfoValue()
	modGain := 1.0 - t.depth*lfo
	wet := x * modGain

	// Advance LFO phase
	t.phase += 2 * math.Pi * t.rate / sr
	if t.phase >= 2*math.Pi {
		t.phase -= 2 * math.Pi
	}

	return x*(1-t.mix) + wet*t.mix
}

func (t *tremolo) Reset() {
	t.phase = 0
}

func (t *tremolo) SetParam(name string, value float64) {
	switch name {
	case "rate":
		t.rate = clampf(value, 0.5, 20)
	case "depth":
		t.depth = clampf(value, 0, 1)
	case "shape":
		t.shape = int(clampf(value, 0, 2))
	case "mix":
		t.mix = clampf(value, 0, 1)
	}
}

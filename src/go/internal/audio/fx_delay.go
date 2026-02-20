//go:build test || js

package audio

import "math"

// delay implements a mono delay line with LP-filtered feedback and wet/dry mix.
type delay struct {
	timeMs   float64 // delay time in ms
	feedback float64 // feedback amount 0-0.95
	mix      float64 // wet/dry 0-1

	sr     int
	buf    []float64 // circular buffer
	pos    int       // write position
	delaySamples int

	// One-pole LP on feedback to darken repeats
	lpY1 float64
	lpA  float64
}

func newDelay(sr int, params map[string]float64) *delay {
	d := &delay{sr: sr}
	d.timeMs = clampf(params["time"], 10, 1000)
	d.feedback = clampf(params["feedback"], 0, 0.95)
	d.mix = clampf(params["mix"], 0, 1)
	d.recalc()
	return d
}

func (d *delay) recalc() {
	sr := float64(d.sr)
	if sr <= 0 {
		sr = 44100
	}
	d.delaySamples = int(math.Round(d.timeMs * 0.001 * sr))
	if d.delaySamples < 1 {
		d.delaySamples = 1
	}
	// Max buffer: 1 second at sample rate
	maxLen := d.sr
	if maxLen <= 0 {
		maxLen = 44100
	}
	if d.delaySamples > maxLen {
		d.delaySamples = maxLen
	}
	// Resize buffer if needed (preserve existing data by allocating fresh)
	needed := d.delaySamples + 1
	if len(d.buf) < needed {
		d.buf = make([]float64, needed)
		d.pos = 0
	}
	// LP for feedback darkening: cutoff at 3 kHz
	d.lpA = math.Exp(-2.0 * math.Pi * 3000.0 / sr)
}

func (d *delay) ProcessSample(x float64) float64 {
	bufLen := len(d.buf)
	if bufLen == 0 {
		return x
	}

	// Read from delay line
	readPos := d.pos - d.delaySamples
	if readPos < 0 {
		readPos += bufLen
	}
	delayed := d.buf[readPos]

	// LP filter on feedback
	d.lpY1 = delayed*(1-d.lpA) + d.lpY1*d.lpA
	filtered := d.lpY1

	// Write input + filtered feedback to buffer
	d.buf[d.pos] = x + filtered*d.feedback

	d.pos++
	if d.pos >= bufLen {
		d.pos = 0
	}

	return x*(1-d.mix) + delayed*d.mix
}

func (d *delay) Reset() {
	for i := range d.buf {
		d.buf[i] = 0
	}
	d.pos = 0
	d.lpY1 = 0
}

func (d *delay) SetParam(name string, value float64) {
	switch name {
	case "time":
		d.timeMs = clampf(value, 10, 1000)
		d.recalc()
	case "feedback":
		d.feedback = clampf(value, 0, 0.95)
	case "mix":
		d.mix = clampf(value, 0, 1)
	}
}

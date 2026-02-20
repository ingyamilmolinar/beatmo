//go:build test || js

package audio

import "math"

// delay implements a mono delay line with LP-filtered feedback and wet/dry mix.
// The feedback and mix parameters are smoothed to prevent clicks. Delay time
// changes use a crossfade between old and new read positions to avoid pops.
type delay struct {
	timeMs   float64 // delay time in ms (not smoothed — crossfaded instead)
	feedback smoothParam
	mix      smoothParam

	sr           int
	buf          []float64 // circular buffer (pre-allocated to max 1s)
	pos          int       // write position
	delaySamples int

	// Crossfade state for delay time changes.
	oldDelaySamples int // previous delay in samples (0 = no crossfade active)
	xfadePos        int // current position in crossfade ramp
	xfadeLen        int // total crossfade length in samples

	// One-pole LP on feedback to darken repeats
	lpY1 float64
	lpA  float64
}

func newDelay(sr int, params map[string]float64) *delay {
	d := &delay{sr: sr}
	d.timeMs = clampf(params["time"], 10, 1000)
	d.feedback = newSmoothParam(clampf(params["feedback"], 0, 0.95), sr, defaultSmoothTimeMs)
	d.mix = newSmoothParam(clampf(params["mix"], 0, 1), sr, defaultSmoothTimeMs)
	d.initBuf()
	return d
}

// initBuf pre-allocates a 1-second buffer and computes initial delay parameters.
func (d *delay) initBuf() {
	sr := float64(d.sr)
	if sr <= 0 {
		sr = 44100
	}

	// Pre-allocate max buffer (1 second) to avoid reallocation on time change.
	maxLen := d.sr
	if maxLen <= 0 {
		maxLen = 44100
	}
	if len(d.buf) < maxLen+1 {
		d.buf = make([]float64, maxLen+1)
		d.pos = 0
	}

	d.delaySamples = int(math.Round(d.timeMs * 0.001 * sr))
	if d.delaySamples < 1 {
		d.delaySamples = 1
	}
	if d.delaySamples > maxLen {
		d.delaySamples = maxLen
	}

	// LP for feedback darkening: cutoff at 3 kHz
	d.lpA = math.Exp(-2.0 * math.Pi * 3000.0 / sr)

	// Crossfade duration: ~5ms
	d.xfadeLen = int(math.Round(sr * defaultSmoothTimeMs * 0.001))
	if d.xfadeLen < 1 {
		d.xfadeLen = 1
	}
}

func (d *delay) readAt(delaySamples int) float64 {
	bufLen := len(d.buf)
	readPos := d.pos - delaySamples
	if readPos < 0 {
		readPos += bufLen
	}
	if readPos >= bufLen {
		readPos = 0
	}
	return d.buf[readPos]
}

func (d *delay) ProcessSample(x float64) float64 {
	bufLen := len(d.buf)
	if bufLen == 0 {
		return x
	}

	fb := d.feedback.tick()
	mix := d.mix.tick()

	// Read from delay line, crossfading if time was recently changed.
	var delayed float64
	if d.xfadePos < d.xfadeLen && d.oldDelaySamples > 0 {
		// Crossfade between old and new read positions.
		t := float64(d.xfadePos) / float64(d.xfadeLen)
		oldVal := d.readAt(d.oldDelaySamples)
		newVal := d.readAt(d.delaySamples)
		delayed = oldVal*(1-t) + newVal*t
		d.xfadePos++
		if d.xfadePos >= d.xfadeLen {
			d.oldDelaySamples = 0 // crossfade complete
		}
	} else {
		delayed = d.readAt(d.delaySamples)
	}

	// LP filter on feedback
	d.lpY1 = delayed*(1-d.lpA) + d.lpY1*d.lpA
	filtered := d.lpY1

	// Write input + filtered feedback to buffer
	d.buf[d.pos] = x + filtered*fb

	d.pos++
	if d.pos >= bufLen {
		d.pos = 0
	}

	return x*(1-mix) + delayed*mix
}

func (d *delay) Reset() {
	for i := range d.buf {
		d.buf[i] = 0
	}
	d.pos = 0
	d.lpY1 = 0
	d.oldDelaySamples = 0
	d.xfadePos = 0
}

func (d *delay) SetParam(name string, value float64) {
	switch name {
	case "time":
		newTimeMs := clampf(value, 10, 1000)
		sr := float64(d.sr)
		if sr <= 0 {
			sr = 44100
		}
		maxLen := d.sr
		if maxLen <= 0 {
			maxLen = 44100
		}
		newDelay := int(math.Round(newTimeMs * 0.001 * sr))
		if newDelay < 1 {
			newDelay = 1
		}
		if newDelay > maxLen {
			newDelay = maxLen
		}
		if newDelay != d.delaySamples {
			d.oldDelaySamples = d.delaySamples
			d.delaySamples = newDelay
			d.xfadePos = 0
		}
		d.timeMs = newTimeMs
	case "feedback":
		d.feedback.set(clampf(value, 0, 0.95))
	case "mix":
		d.mix.set(clampf(value, 0, 1))
	}
}

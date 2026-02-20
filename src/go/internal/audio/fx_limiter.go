//go:build test || js

package audio

import "math"

// limiter implements a brick-wall limiter (infinite-ratio compressor) with
// instant attack and configurable release. Output is guaranteed never to
// exceed the ceiling level.
type limiter struct {
	thresholdDB float64 // -20 to 0 dB
	releaseMs   float64 // 1 to 500 ms
	ceilingDB   float64 // -6 to 0 dB

	sr           int
	envelope     float64
	releaseCoef  float64
	thresholdLin float64
	ceilingLin   float64
}

func newLimiter(sr int, params map[string]float64) *limiter {
	l := &limiter{sr: sr}
	l.thresholdDB = clampf(params["threshold"], -20, 0)
	l.releaseMs = clampf(params["release"], 1, 500)
	l.ceilingDB = clampf(params["ceiling"], -6, 0)
	l.recalc()
	return l
}

func (l *limiter) recalc() {
	sr := float64(l.sr)
	if sr <= 0 {
		sr = 44100
	}
	l.releaseCoef = math.Exp(-1.0 / (l.releaseMs * 0.001 * sr))
	l.thresholdLin = math.Pow(10, l.thresholdDB/20)
	l.ceilingLin = math.Pow(10, l.ceilingDB/20)
}

func (l *limiter) ProcessSample(x float64) float64 {
	absX := math.Abs(x)

	// Instant attack: envelope immediately follows peaks above current level.
	if absX > l.envelope {
		l.envelope = absX
	} else {
		l.envelope = l.releaseCoef*l.envelope + (1-l.releaseCoef)*absX
	}

	// Gain reduction: brick-wall at threshold.
	gain := 1.0
	if l.envelope > l.thresholdLin {
		gain = l.thresholdLin / l.envelope
	}

	// Scale output to ceiling.
	return x * gain * (l.ceilingLin / l.thresholdLin)
}

func (l *limiter) Reset() {
	l.envelope = 0
}

func (l *limiter) SetParam(name string, value float64) {
	switch name {
	case "threshold":
		l.thresholdDB = clampf(value, -20, 0)
		l.recalc()
	case "release":
		l.releaseMs = clampf(value, 1, 500)
		l.recalc()
	case "ceiling":
		l.ceilingDB = clampf(value, -6, 0)
		l.recalc()
	}
}

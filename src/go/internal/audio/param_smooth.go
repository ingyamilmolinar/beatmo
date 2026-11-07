//go:build test || js

package audio

import "math"

// A one-pole ramp toward each new target over ~smoothTimeMs: an instant change clicks or zippers.
type smoothParam struct {
	target  float64
	current float64
	coeff   float64 // smoothing coefficient per sample
}

const defaultSmoothTimeMs = 5.0

// Starts at initial, with no ramp from zero.
func newSmoothParam(initial float64, sr int, timeMs float64) smoothParam {
	c := 1.0
	if sr > 0 && timeMs > 0 {
		// At 44.1 kHz, 5 ms ≈ 220 samples, coeff ≈ 0.0045.
		samples := float64(sr) * timeMs * 0.001
		c = 1.0 - math.Exp(-1.0/samples)
	}
	return smoothParam{
		target:  initial,
		current: initial,
		coeff:   c,
	}
}

func (s *smoothParam) set(v float64) {
	s.target = v
}

// For Reset(), so nothing ramps from a stale value.
func (s *smoothParam) snap(v float64) {
	s.target = v
	s.current = v
}

// tick advances the smoother by one sample and returns the smoothed value.
// Call this once per sample in ProcessSample.
func (s *smoothParam) tick() float64 {
	s.current += (s.target - s.current) * s.coeff
	return s.current
}

// value returns the current smoothed value without advancing.
func (s *smoothParam) value() float64 {
	return s.current
}

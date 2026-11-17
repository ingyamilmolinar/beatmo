//go:build test || js

package audio

import "math"

// smoothParam implements a one-pole exponential smoother for audio effect
// parameters. When a parameter is changed via set(), the value ramps smoothly
// from the current value to the target over approximately smoothTimeMs,
// eliminating the clicks and zipper noise caused by instantaneous changes.
//
// Usage:
//
//	p := newSmoothParam(initialValue, sampleRate, 5.0) // 5ms smoothing
//	p.set(newValue)                                     // called from SetParam
//	smoothed := p.tick()                                // called per sample in ProcessSample
type smoothParam struct {
	target  float64
	current float64
	coeff   float64 // smoothing coefficient per sample
}

const defaultSmoothTimeMs = 5.0

// newSmoothParam creates a smoother initialized to initial, with the given
// sample rate and smoothing time in milliseconds. The initial value is set
// immediately (no ramp from zero).
func newSmoothParam(initial float64, sr int, timeMs float64) smoothParam {
	c := 1.0
	if sr > 0 && timeMs > 0 {
		// One-pole coefficient: how much of the error to close per sample.
		// At 44100 Hz, 5ms → ~220 samples → coeff ≈ 0.0045.
		samples := float64(sr) * timeMs * 0.001
		c = 1.0 - math.Exp(-1.0/samples)
	}
	return smoothParam{
		target:  initial,
		current: initial,
		coeff:   c,
	}
}

// set updates the target value. The current value will ramp toward it over
// the configured smoothing time.
func (s *smoothParam) set(v float64) {
	s.target = v
}

// snap immediately sets both current and target to v, bypassing smoothing.
// Used during Reset() to avoid ramping from stale values.
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

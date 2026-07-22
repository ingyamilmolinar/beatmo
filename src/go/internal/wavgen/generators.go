package wavgen

import "math"

// Sine returns durSec of a mono sine wave at freqHz, amplitude amp ([0,1]).
func Sine(freqHz, amp, durSec float64, sampleRate int) []float32 {
	n := int(durSec * float64(sampleRate))
	out := make([]float32, n)
	for i := range out {
		t := float64(i) / float64(sampleRate)
		out[i] = float32(amp * math.Sin(2*math.Pi*freqHz*t))
	}
	return out
}

// Noise returns durSec of deterministic mono white noise at amplitude amp,
// seeded so tests are reproducible (no Math.random / time dependence — a fixed
// seed always yields identical samples).
func Noise(amp, durSec float64, sampleRate, seed int) []float32 {
	n := int(durSec * float64(sampleRate))
	out := make([]float32, n)
	st := uint32(seed)*2654435761 + 1
	for i := range out {
		st = st*1664525 + 1013904223 // LCG
		f := float64(st>>8)/float64(1<<24)*2 - 1
		out[i] = float32(amp * f)
	}
	return out
}

package ui

import "math"

// Every animation constant comes from a generated genAnim* token (edit DESIGN.md `animations:` and run
// `make gen-design-tokens`). The helpers inline to the same code as the raw math.

// false once below the threshold: the caller drops the entry.
func DecayStep(v float64, anim ExpDecayAnim) (float64, bool) {
	v *= anim.Rate
	if v < anim.Threshold {
		return 0, false
	}
	return v, true
}

// base + amplitude·|sin(frame·step)|. frame is int64 so monotonic counters need no widening.
func SinPulse(frame int64, anim SinPulseAnim) float64 {
	return anim.Base + anim.Amplitude*math.Abs(math.Sin(float64(frame)*anim.FrameStep))
}

func SinPulseAlpha(frame int64, anim SinPulseAnim) uint8 {
	v := SinPulse(frame, anim) * float64(anim.AlphaScale)
	if v < 0 {
		v = 0
	}
	if v > 255 {
		v = 255
	}
	return uint8(v)
}

package clone

import "math/rand"

// modalSignal is the known-answer generator: preroll seconds of noise, then
// the modes (onset at exactly preroll), plus white noise of noiseRMS
// throughout, seeded.
func modalSignal(modes []Mode, rate int, preroll, dur, noiseRMS float64, seed int64) []float64 {
	pre := int(preroll * float64(rate))
	n := int(dur * float64(rate))
	out := make([]float64, pre+n)
	copy(out[pre:], Synthesize(modes, rate, n))
	if noiseRMS > 0 {
		r := rand.New(rand.NewSource(seed))
		for i := range out {
			out[i] += noiseRMS * r.NormFloat64()
		}
	}
	return out
}

// barModes is a struck-bar-like set: inharmonic, higher modes decay faster.
func barModes() []Mode {
	return []Mode{
		{Freq: 440, Alpha: 2.0, Amp: 0.40, Phase: 0.3},
		{Freq: 1210, Alpha: 4.5, Amp: 0.20, Phase: -1.1},
		{Freq: 2380, Alpha: 9.0, Amp: 0.12, Phase: 2.0},
		{Freq: 3950, Alpha: 16, Amp: 0.06, Phase: 0.7},
	}
}

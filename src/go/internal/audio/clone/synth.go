package clone

import (
	"math"
	"math/cmplx"
)

// Synthesize renders modes from t = 0 for n samples at rate: the pure-Go
// reference resynthesis the residual and the tests use (the engine's C modal
// bank is plan 2). One phasor recursion per mode, stopped once the mode is
// 160 dB below its start.
func Synthesize(modes []Mode, rate, n int) []float64 {
	out := make([]float64, n)
	for _, m := range modes {
		if m.Amp == 0 {
			continue
		}
		z := cmplx.Exp(complex(-m.Alpha/float64(rate), 2*math.Pi*m.Freq/float64(rate)))
		s := cmplx.Rect(m.Amp, m.Phase)
		stop := m.Amp * 1e-8
		for i := 0; i < n; i++ {
			out[i] += real(s)
			s *= z
			if i&1023 == 1023 && cmplx.Abs(s) < stop {
				break
			}
		}
	}
	return out
}

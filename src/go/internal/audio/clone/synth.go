package clone

import (
	"math"
	"math/cmplx"
)

// The pure-Go reference resynthesis; each mode stops once 160 dB below its start.
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

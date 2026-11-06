package clone

import (
	"math"
	"math/cmplx"
)

// Sub-band layout (spec §2b): 24 log-spaced bands from 30 Hz to
// min(20 kHz, 0.45·rate). A mode belongs to the band whose CORE [Lo, Hi)
// holds its frequency; each band heterodynes its core centre to DC,
// low-passes and decimates so ESPRIT sees few poles.
//
// Decimated rate fs_d ≥ 2·w (w = core width). The low-pass passes ±0.5w flat
// and stops from ±1.5w (Blackman, cutoff w): a component at |f| ≥ 1.5w is the
// only thing that could fold into the core ±0.5w, and it is ≥ 74 dB down.
type band struct {
	Index  int
	Lo, Hi float64 // core edges, Hz
	Center float64 // heterodyne frequency, Hz
	Decim  int
	Taps   []float64 // zero-phase low-pass, odd length, centred, unit DC gain
}

const (
	bandCount = 24
	bandLoHz  = 30.0
	bandHiHz  = 20000.0
)

func bandLayout(rate int) []band {
	top := math.Min(bandHiHz, 0.45*float64(rate))
	ratio := math.Pow(top/bandLoHz, 1/float64(bandCount))
	out := make([]band, 0, bandCount)
	lo := bandLoHz
	for k := 0; k < bandCount; k++ {
		hi := bandLoHz * math.Pow(ratio, float64(k+1))
		if k == bandCount-1 {
			hi = top
		}
		w := hi - lo
		d := int(float64(rate) / (2 * w))
		if d < 1 {
			d = 1
		}
		out = append(out, band{Index: k, Lo: lo, Hi: hi, Center: (lo + hi) / 2, Decim: d, Taps: lowpassTaps(rate, w)})
		lo = hi
	}
	return out
}

// lowpassTaps is a Blackman-windowed sinc with cutoff w Hz and transition
// width w (0.5w → 1.5w), normalised to unit DC gain.
func lowpassTaps(rate int, w float64) []float64 {
	n := int(math.Ceil(5.5 * float64(rate) / w))
	if n%2 == 0 {
		n++
	}
	if n < 3 {
		n = 3
	}
	fc := w / float64(rate)
	half := n / 2
	h := make([]float64, n)
	var sum float64
	for j := 0; j < n; j++ {
		k := float64(j - half)
		s := 2 * fc
		if k != 0 {
			s = math.Sin(2*math.Pi*fc*k) / (math.Pi * k)
		}
		ph := 2 * math.Pi * float64(j) / float64(n-1)
		h[j] = s * (0.42 - 0.5*math.Cos(ph) + 0.08*math.Cos(2*ph))
		sum += h[j]
	}
	for j := range h {
		h[j] /= sum
	}
	return h
}

// baseband heterodynes x by e^{−i2π·Center·(n−onset)/rate}, low-passes and
// decimates it, using only samples in [from, to) (each output's whole filter
// support lies inside). first is the absolute sample index of y[0]; y[m] is
// centred on first + m·Decim.
func baseband(x []float64, rate, onset, from, to int, b band) ([]complex128, int) {
	half := len(b.Taps) / 2
	first := from + half
	if first+half >= to {
		return nil, 0
	}
	re := make([]float64, to-from)
	im := make([]float64, to-from)
	// e^{−iw(n−onset)} by phasor recursion, re-anchored exactly every 1024
	// samples (drift stays ~1e-13; math.Sincos per sample was a third of the
	// whole analysis).
	w := 2 * math.Pi * b.Center / float64(rate)
	sw, cw := math.Sincos(-w)
	var pr, pi float64
	for n := from; n < to; n++ {
		if (n-from)&1023 == 0 {
			s, c := math.Sincos(-w * float64(n-onset))
			pr, pi = c, s
		}
		re[n-from] = x[n] * pr
		im[n-from] = x[n] * pi
		pr, pi = pr*cw-pi*sw, pr*sw+pi*cw
	}
	var y []complex128
	for c := first; c+half < to; c += b.Decim {
		base := c - half - from
		var ar, ai float64
		for j, h := range b.Taps {
			ar += h * re[base+j]
			ai += h * im[base+j]
		}
		y = append(y, complex(ar, ai))
	}
	return y, first
}

// gain is the band filter's exact response to a baseband component
// e^{s·t} (s = −α + i·2π·Δf, continuous time): Σ h_j e^{s·τ_j}. Dividing a
// measured amplitude by it removes both the pass-band shape and the bias a
// long symmetric filter puts on a DAMPED exponential.
func (b band) gain(s complex128, rate int) complex128 {
	half := len(b.Taps) / 2
	var g complex128
	for j, h := range b.Taps {
		tau := float64(j-half) / float64(rate)
		g += complex(h, 0) * cmplx.Exp(s*complex(tau, 0))
	}
	return g
}

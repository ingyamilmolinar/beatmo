//go:build !test && !js

package audio

import (
	"math"
)
// pow2 computes 2^x.
func pow2(x float64) float64 { return math.Pow(2, x) }

// resampleVoice wraps a source Voice and produces samples at a fractional
// rate using linear interpolation.
type resampleVoice struct {
	src  Voice
	step float64
	pos  float64
	buf  []float64
	done bool
}

func (r *resampleVoice) Sample() (float64, bool) {
	if r.step <= 0 {
		r.step = 1
	}
	// Ensure we have enough source samples to interpolate at current pos.
	need := int(r.pos) + 2
	for !r.done && len(r.buf) < need {
		s, d := r.src.Sample()
		r.buf = append(r.buf, s)
		if d {
			r.done = true
			break
		}
	}
	if len(r.buf) == 0 {
		return 0, true
	}
	i0 := int(r.pos)
	if i0 >= len(r.buf)-1 {
		if r.done {
			return 0, true
		}
		// Try to fetch one more sample.
		s, d := r.src.Sample()
		r.buf = append(r.buf, s)
		if d {
			r.done = true
		}
		if i0 >= len(r.buf)-1 && r.done {
			return 0, true
		}
	}
	var s0, s1 float64
	s0 = r.buf[i0]
	if i0+1 < len(r.buf) {
		s1 = r.buf[i0+1]
	} else {
		s1 = 0
	}
	frac := r.pos - float64(i0)
	out := s0 + (s1-s0)*frac
	r.pos += r.step
	return out, false
}


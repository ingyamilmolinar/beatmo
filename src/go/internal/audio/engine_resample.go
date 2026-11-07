//go:build !test && !js

package audio

import (
	"math"
)

// pow2 computes 2^x.
func pow2(x float64) float64 { return math.Pow(2, x) }

// Output sample k is the source linearly interpolated at k·step (silent past its end) for every k with k·step < its
// length: the browser builds a pitched drum with the same arithmetic, so a note is the same samples on both. k·step,
// never a running sum, so positions do not drift.
type resampleVoice struct {
	src  Voice
	step float64
	k    int
	buf  []float64
	done bool // src exhausted
}

// It never reads ahead: probing for the end by calling Sample consumes one output sample per block.
func (r *resampleVoice) SampleBlock(dst []float64) (int, bool) {
	for i := range dst {
		val, done := r.Sample()
		if done {
			return i, true
		}
		dst[i] = val
	}
	return len(dst), false
}

func (r *resampleVoice) Sample() (float64, bool) {
	if r.step <= 0 {
		r.step = 1
	}
	pos := float64(r.k) * r.step
	i0 := int(pos)
	for !r.done && len(r.buf) < i0+2 {
		s, d := r.src.Sample()
		if d {
			r.done = true
			break
		}
		r.buf = append(r.buf, s)
	}
	n := len(r.buf)
	if i0 >= n {
		return 0, true
	}
	s0, s1 := r.buf[i0], 0.0
	if i0+1 < n {
		s1 = r.buf[i0+1]
	}
	r.k++
	return s0 + (s1-s0)*(pos-float64(i0)), false
}

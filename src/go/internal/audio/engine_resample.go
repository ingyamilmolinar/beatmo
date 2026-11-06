//go:build !test && !js

package audio

import (
	"math"
)

// pow2 computes 2^x.
func pow2(x float64) float64 { return math.Pow(2, x) }

// resampleVoice plays its source at a fractional rate: output sample k is the
// source linearly interpolated at position k·step, the source counting as
// silent past its end, for every k with k·step < its length. That is the
// whole definition — the browser builds a pitched drum's buffer with the same
// arithmetic (played_voice.js resampleVoiceBuffer), so a note is the same
// samples on both platforms. Positions are k·step, never a running sum, so
// they do not drift over a long voice.
type resampleVoice struct {
	src  Voice
	step float64
	k    int
	buf  []float64
	done bool // src exhausted
}

// SampleBlock implements BlockVoice. It never reads ahead: the old version
// "probed" for the end after every block by calling Sample, which consumed —
// and dropped — one output sample per 64-frame mixer block.
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

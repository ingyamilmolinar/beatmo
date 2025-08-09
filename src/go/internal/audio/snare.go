//go:build !test

package audio

import (
	"math"
	"math/rand"
	"time"
)

// Snare mixes filtered noise with a resonant tone for a sharper drum hit.
type Snare struct{}

// NewVoice returns a decaying snare hit whose length depends on BPM.
func (Snare) NewVoice(bpm, sampleRate int) Voice {
	spb := 60 / float64(bpm)
	dur := time.Duration(spb * 0.25 * float64(time.Second))
	samples := int(float64(sampleRate) * dur.Seconds())
	return &snareVoice{
		n:   samples,
		sr:  float64(sampleRate),
		rnd: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

type snareVoice struct {
	i, n      int
	sr        float64
	rnd       *rand.Rand
	lp, phase float64
}

func (s *snareVoice) Sample() (float64, bool) {
	if s.i >= s.n {
		return 0, true
	}
	t := float64(s.i) / float64(s.n)
	white := s.rnd.Float64()*2 - 1
	s.lp = s.lp*0.7 + white*0.3 // simple low-pass
	noise := s.lp * math.Exp(-6*t)
	freq := 200.0 - 60.0*t
	s.phase += 2 * math.Pi * freq / s.sr
	tone := math.Sin(s.phase) * math.Exp(-4*t)
	v := noise*0.6 + tone*0.4
	s.i++
	return v, false
}

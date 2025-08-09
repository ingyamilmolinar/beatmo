//go:build !test

package audio

import (
	"math"
	"math/rand"
	"time"
)

// Snare is a white-noise drum instrument with an exponential decay envelope.
type Snare struct{}

// NewVoice returns a decaying noise burst whose length depends on BPM.
func (Snare) NewVoice(bpm, sampleRate int) Voice {
	spb := 60 / float64(bpm)
	dur := time.Duration(spb * 0.5 * float64(time.Second))
	samples := int(float64(sampleRate) * dur.Seconds())
	return &noiseVoice{
		remaining: samples,
		rnd:       rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

type noiseVoice struct {
	i, remaining int
	rnd          *rand.Rand
}

func (n *noiseVoice) Sample() (float64, bool) {
	if n.i >= n.remaining {
		return 0, true
	}
	env := math.Exp(-4 * float64(n.i) / float64(n.remaining))
	v := (n.rnd.Float64()*2 - 1) * env
	n.i++
	return v, false
}

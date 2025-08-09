//go:build !test && !js

package audio

import (
	"math"
	"time"
)

// Kick is a simple sine-based bass drum.
type Kick struct{}

// NewVoice returns a decaying sine with a downward pitch bend.
func (Kick) NewVoice(bpm, sampleRate int) Voice {
	spb := 60 / float64(bpm)
	dur := time.Duration(spb * 0.5 * float64(time.Second))
	samples := int(float64(sampleRate) * dur.Seconds())
	return &kickVoice{n: samples, sr: float64(sampleRate)}
}

type kickVoice struct {
	i, n  int
	sr    float64
	phase float64
}

func (k *kickVoice) Sample() (float64, bool) {
	if k.i >= k.n {
		return 0, true
	}
	t := float64(k.i) / float64(k.n)
	freq := 150 - 100*t
	k.phase += 2 * math.Pi * freq / k.sr
	env := math.Exp(-5 * t)
	v := math.Sin(k.phase) * env
	k.i++
	return v, false
}

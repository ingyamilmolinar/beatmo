//go:build !test && !js

package audio

import "time"

// HiHat renders a short, bright noise burst.
// It aims to mimic a closed hi-hat.
type HiHat struct{}

// NewVoice generates a hi-hat hit via the C renderer.
func (HiHat) NewVoice(bpm, sampleRate int) Voice {
	spb := 60 / float64(bpm)
	dur := time.Duration(spb * 0.125 * float64(time.Second))
	samples := int(float64(sampleRate) * dur.Seconds())
	buf := make([]float32, samples)
	renderHiHat(buf, sampleRate, samples)
	return &cVoice{buf: buf}
}

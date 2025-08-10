//go:build !test && !js

package audio

import "time"

// Clap renders multiple short noise bursts for a hand clap.
type Clap struct{}

// NewVoice generates a clap hit via the C renderer.
func (Clap) NewVoice(bpm, sampleRate int) Voice {
	spb := 60 / float64(bpm)
	dur := time.Duration(spb * 0.25 * float64(time.Second))
	samples := int(float64(sampleRate) * dur.Seconds())
	buf := make([]float32, samples)
	renderClap(buf, sampleRate, samples)
	return &cVoice{buf: buf}
}

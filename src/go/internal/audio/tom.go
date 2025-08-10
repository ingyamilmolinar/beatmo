//go:build !test && !js

package audio

import "time"

// Tom renders a pitched drum tone with a slight noise attack.
type Tom struct{}

// NewVoice generates a tom hit via the C renderer.
func (Tom) NewVoice(bpm, sampleRate int) Voice {
	spb := 60 / float64(bpm)
	dur := time.Duration(spb * 0.5 * float64(time.Second))
	samples := int(float64(sampleRate) * dur.Seconds())
	buf := make([]float32, samples)
	renderTom(buf, sampleRate, samples)
	return &cVoice{buf: buf}
}

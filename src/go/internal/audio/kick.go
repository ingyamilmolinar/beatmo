//go:build !test && !js

package audio

import "time"

// Kick renders a bass drum via Miniaudio.
type Kick struct{}

// NewVoice generates a kick hit via the C renderer.
func (Kick) NewVoice(bpm, sampleRate int) Voice {
	spb := 60 / float64(bpm)
	dur := time.Duration(spb * 0.5 * float64(time.Second))
	samples := int(float64(sampleRate) * dur.Seconds())
	buf := make([]float32, samples)
	renderKick(buf, sampleRate, samples)
	return &cVoice{buf: buf}
}

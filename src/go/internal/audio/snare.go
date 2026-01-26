//go:build !test && !js

package audio

import "time"

// Snare renders white noise shaped by Miniaudio.
type Snare struct{}

// NewVoice generates a snare hit via the C renderer.
func (Snare) NewVoice(bpm, sampleRate int) Voice {
	spb := 60 / float64(bpm)
	// Longer snare to allow a rock-style ring in rock grooves.
	dur := time.Duration(spb * 1.0 * float64(time.Second))
	samples := int(float64(sampleRate) * dur.Seconds())
	buf := make([]float32, samples)
	renderSnare(buf, sampleRate, samples)
	return &cVoice{buf: buf}
}

//go:build !test && !js

package audio

import "time"

// Cowbell renders a synthetic cowbell tone using the C renderer.
type Cowbell struct{}

// NewVoice generates a cowbell hit via the C renderer.
func (Cowbell) NewVoice(bpm, sampleRate int) Voice {
	spb := 60 / float64(bpm)
	dur := time.Duration(spb * 0.5 * float64(time.Second))
	samples := int(float64(sampleRate) * dur.Seconds())
	buf := make([]float32, samples)
	renderCowbell(buf, sampleRate, samples)
	return &cVoice{buf: buf}
}

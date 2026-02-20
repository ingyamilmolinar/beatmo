//go:build !test && !js

package audio

// Shaker renders a noise-based shaker with grain-like micro-bursts.
type Shaker struct{}

// NewVoice generates a shaker hit via the C renderer.
func (Shaker) NewVoice(bpm, sampleRate int) Voice {
	key := voiceCacheKey{instrumentID: "shaker", sampleRate: sampleRate}
	if buf, ok := globalVoiceCache.Get(key); ok {
		return &cVoice{buf: buf}
	}
	cfg := ConfigForInstrument("shaker")
	samples := int(float64(sampleRate) * cfg.DurationSec)
	buf := make([]float32, samples)
	renderShaker(buf, sampleRate, samples)
	normalizeAndScale(buf, "shaker")
	globalVoiceCache.Put(key, buf)
	return &cVoice{buf: buf}
}

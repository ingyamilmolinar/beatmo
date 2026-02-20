//go:build !test && !js

package audio

// Sidestick renders a dry cross-stick click via the C renderer.
type Sidestick struct{}

// NewVoice generates a sidestick hit via the C renderer.
func (Sidestick) NewVoice(bpm, sampleRate int) Voice {
	key := voiceCacheKey{instrumentID: "sidestick", sampleRate: sampleRate}
	if buf, ok := globalVoiceCache.Get(key); ok {
		return &cVoice{buf: buf}
	}
	cfg := ConfigForInstrument("sidestick")
	samples := int(float64(sampleRate) * cfg.DurationSec)
	buf := make([]float32, samples)
	renderSnareSidestick(buf, sampleRate, samples)
	normalizeAndScale(buf, "sidestick")
	globalVoiceCache.Put(key, buf)
	return &cVoice{buf: buf}
}

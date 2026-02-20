//go:build !test && !js

package audio

// Rimshot renders a cracking rimshot via the C renderer.
type Rimshot struct{}

// NewVoice generates a rimshot hit via the C renderer.
func (Rimshot) NewVoice(bpm, sampleRate int) Voice {
	key := voiceCacheKey{instrumentID: "rimshot", sampleRate: sampleRate}
	if buf, ok := globalVoiceCache.Get(key); ok {
		return &cVoice{buf: buf}
	}
	cfg := ConfigForInstrument("rimshot")
	samples := int(float64(sampleRate) * cfg.DurationSec)
	buf := make([]float32, samples)
	renderSnareRimshot(buf, sampleRate, samples)
	normalizeAndScale(buf, "rimshot")
	globalVoiceCache.Put(key, buf)
	return &cVoice{buf: buf}
}

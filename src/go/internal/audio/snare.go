//go:build !test && !js

package audio

// Snare renders white noise shaped by Miniaudio.
type Snare struct{}

// NewVoice generates a snare hit via the C renderer.
// Uses voice cache to avoid redundant CGo calls for identical parameters.
func (Snare) NewVoice(bpm, sampleRate int) Voice {
	key := voiceCacheKey{instrumentID: "snare", sampleRate: sampleRate}
	if buf, ok := globalVoiceCache.Get(key); ok {
		return &cVoice{buf: buf}
	}
	cfg := ConfigForInstrument("snare")
	samples := int(float64(sampleRate) * cfg.DurationSec)
	buf := make([]float32, samples)
	renderSnare(buf, sampleRate, samples)
	normalizeAndScale(buf, "snare")
	globalVoiceCache.Put(key, buf)
	return &cVoice{buf: buf}
}

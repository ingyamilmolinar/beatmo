//go:build !test && !js

package audio

// Tom renders a pitched drum tone with a slight noise attack.
type Tom struct{}

// NewVoice generates a tom hit via the C renderer.
// Uses voice cache to avoid redundant CGo calls for identical parameters.
func (Tom) NewVoice(bpm, sampleRate int) Voice {
	key := voiceCacheKey{instrumentID: "tom", sampleRate: sampleRate}
	if buf, ok := globalVoiceCache.Get(key); ok {
		return &cVoice{buf: buf}
	}
	cfg := ConfigForInstrument("tom")
	samples := int(float64(sampleRate) * cfg.DurationSec)
	buf := make([]float32, samples)
	renderTom(buf, sampleRate, samples)
	normalizeAndScale(buf, "tom")
	globalVoiceCache.Put(key, buf)
	return &cVoice{buf: buf}
}

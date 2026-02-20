//go:build !test && !js

package audio

// Kick renders a bass drum via Miniaudio.
type Kick struct{}

// NewVoice generates a kick hit via the C renderer.
// Uses voice cache to avoid redundant CGo calls for identical parameters.
func (Kick) NewVoice(bpm, sampleRate int) Voice {
	key := voiceCacheKey{instrumentID: "kick", sampleRate: sampleRate}
	if buf, ok := globalVoiceCache.Get(key); ok {
		return &cVoice{buf: buf}
	}
	cfg := ConfigForInstrument("kick")
	samples := int(float64(sampleRate) * cfg.DurationSec)
	buf := make([]float32, samples)
	renderKick(buf, sampleRate, samples)
	normalizeAndScale(buf, "kick")
	globalVoiceCache.Put(key, buf)
	return &cVoice{buf: buf}
}

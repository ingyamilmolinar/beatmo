//go:build !test && !js

package audio

// Cowbell renders a synthetic cowbell tone using the C renderer.
type Cowbell struct{}

// NewVoice generates a cowbell hit via the C renderer.
// Uses voice cache to avoid redundant CGo calls for identical parameters.
func (Cowbell) NewVoice(bpm, sampleRate int) Voice {
	key := voiceCacheKey{instrumentID: "cowbell", sampleRate: sampleRate}
	if buf, ok := globalVoiceCache.Get(key); ok {
		return &cVoice{buf: buf}
	}
	cfg := ConfigForInstrument("cowbell")
	samples := int(float64(sampleRate) * cfg.DurationSec)
	buf := make([]float32, samples)
	renderCowbellVoice(buf, sampleRate, samples) // modular fast path (Phase-6 cutover)
	normalizeAndScale(buf, "cowbell")
	globalVoiceCache.Put(key, buf)
	return &cVoice{buf: buf}
}

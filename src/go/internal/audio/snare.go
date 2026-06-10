//go:build !test && !js

package audio

// Snare renders the snare voice through the modular engine (Phase-5 migration).
type Snare struct{}

// NewVoice generates a snare hit via the modular no-edit fast path.
// Uses voice cache to avoid redundant CGo calls for identical parameters.
func (Snare) NewVoice(bpm, sampleRate int) Voice {
	key := voiceCacheKey{instrumentID: "snare", sampleRate: sampleRate}
	if buf, ok := globalVoiceCache.Get(key); ok {
		return &cVoice{buf: buf}
	}
	cfg := ConfigForInstrument("snare")
	samples := int(float64(sampleRate) * cfg.DurationSec)
	buf := make([]float32, samples)
	renderSnareVoice(buf, sampleRate, samples) // modular fast path (Phase-5 cutover)
	normalizeAndScale(buf, "snare")
	globalVoiceCache.Put(key, buf)
	return &cVoice{buf: buf}
}

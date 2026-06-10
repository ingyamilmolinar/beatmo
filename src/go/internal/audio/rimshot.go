//go:build !test && !js

package audio

// Rimshot renders a cracking rimshot through the modular engine (Phase-5).
type Rimshot struct{}

// NewVoice generates a rimshot hit via the modular no-edit fast path.
func (Rimshot) NewVoice(bpm, sampleRate int) Voice {
	key := voiceCacheKey{instrumentID: "rimshot", sampleRate: sampleRate}
	if buf, ok := globalVoiceCache.Get(key); ok {
		return &cVoice{buf: buf}
	}
	cfg := ConfigForInstrument("rimshot")
	samples := int(float64(sampleRate) * cfg.DurationSec)
	buf := make([]float32, samples)
	renderSnareRimshotVoice(buf, sampleRate, samples) // modular fast path (Phase-5 cutover)
	normalizeAndScale(buf, "rimshot")
	globalVoiceCache.Put(key, buf)
	return &cVoice{buf: buf}
}

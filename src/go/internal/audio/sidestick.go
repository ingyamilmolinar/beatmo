//go:build !test && !js

package audio

// Sidestick renders a dry cross-stick click through the modular engine (Phase-5).
type Sidestick struct{}

// NewVoice generates a sidestick hit via the modular no-edit fast path.
func (Sidestick) NewVoice(bpm, sampleRate int) Voice {
	key := voiceCacheKey{instrumentID: "sidestick", sampleRate: sampleRate}
	if buf, ok := globalVoiceCache.Get(key); ok {
		return &cVoice{buf: buf}
	}
	cfg := ConfigForInstrument("sidestick")
	samples := int(float64(sampleRate) * cfg.DurationSec)
	buf := make([]float32, samples)
	renderSnareSidestickVoice(buf, sampleRate, samples) // modular fast path (Phase-5 cutover)
	normalizeAndScale(buf, "sidestick")
	globalVoiceCache.Put(key, buf)
	return &cVoice{buf: buf}
}

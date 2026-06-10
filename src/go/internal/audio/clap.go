//go:build !test && !js

package audio

// Clap renders the clap voice through the modular engine (Phase-5 migration).
type Clap struct{}

// NewVoice generates a clap hit via the modular no-edit fast path.
// Uses voice cache to avoid redundant CGo calls for identical parameters.
func (Clap) NewVoice(bpm, sampleRate int) Voice {
	key := voiceCacheKey{instrumentID: "clap", sampleRate: sampleRate}
	if buf, ok := globalVoiceCache.Get(key); ok {
		return &cVoice{buf: buf}
	}
	cfg := ConfigForInstrument("clap")
	samples := int(float64(sampleRate) * cfg.DurationSec)
	buf := make([]float32, samples)
	renderClapVoice(buf, sampleRate, samples) // modular fast path (Phase-5 cutover)
	normalizeAndScale(buf, "clap")
	globalVoiceCache.Put(key, buf)
	return &cVoice{buf: buf}
}

//go:build !test && !js

package audio

// Clap renders multiple short noise bursts for a hand clap.
type Clap struct{}

// NewVoice generates a clap hit via the C renderer.
// Uses voice cache to avoid redundant CGo calls for identical parameters.
func (Clap) NewVoice(bpm, sampleRate int) Voice {
	key := voiceCacheKey{instrumentID: "clap", sampleRate: sampleRate}
	if buf, ok := globalVoiceCache.Get(key); ok {
		return &cVoice{buf: buf}
	}
	cfg := ConfigForInstrument("clap")
	samples := int(float64(sampleRate) * cfg.DurationSec)
	buf := make([]float32, samples)
	renderClap(buf, sampleRate, samples)
	normalizeAndScale(buf, "clap")
	globalVoiceCache.Put(key, buf)
	return &cVoice{buf: buf}
}

//go:build !test && !js

package audio

// HiHat renders a short, bright noise burst.
// It aims to mimic a closed hi-hat.
type HiHat struct{}

// NewVoice generates a hi-hat hit via the C renderer.
// Uses voice cache to avoid redundant CGo calls for identical parameters.
func (HiHat) NewVoice(bpm, sampleRate int) Voice {
	key := voiceCacheKey{instrumentID: "hihat", sampleRate: sampleRate}
	if buf, ok := globalVoiceCache.Get(key); ok {
		return &cVoice{buf: buf}
	}
	cfg := ConfigForInstrument("hihat")
	samples := int(float64(sampleRate) * cfg.DurationSec)
	buf := make([]float32, samples)
	renderHiHat(buf, sampleRate, samples)
	normalizeAndScale(buf, "hihat")
	globalVoiceCache.Put(key, buf)
	return &cVoice{buf: buf}
}

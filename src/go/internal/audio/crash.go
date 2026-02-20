//go:build !test && !js

package audio

// Crash renders a wide-band crash cymbal with long decay.
type Crash struct{}

// NewVoice generates a crash cymbal hit via the C renderer.
func (Crash) NewVoice(bpm, sampleRate int) Voice {
	key := voiceCacheKey{instrumentID: "crash", sampleRate: sampleRate}
	if buf, ok := globalVoiceCache.Get(key); ok {
		return &cVoice{buf: buf}
	}
	cfg := ConfigForInstrument("crash")
	samples := int(float64(sampleRate) * cfg.DurationSec)
	buf := make([]float32, samples)
	renderCrash(buf, sampleRate, samples)
	normalizeAndScale(buf, "crash")
	globalVoiceCache.Put(key, buf)
	return &cVoice{buf: buf}
}

//go:build !test && !js

package audio

// KickDeep renders an 808-style deep sub kick via the C renderer.
type KickDeep struct{}

// NewVoice generates a deep kick hit via the C renderer.
func (KickDeep) NewVoice(bpm, sampleRate int) Voice {
	key := voiceCacheKey{instrumentID: "kick-deep", sampleRate: sampleRate}
	if buf, ok := globalVoiceCache.Get(key); ok {
		return &cVoice{buf: buf}
	}
	cfg := ConfigForInstrument("kick-deep")
	samples := int(float64(sampleRate) * cfg.DurationSec)
	buf := make([]float32, samples)
	renderKickDeep(buf, sampleRate, samples)
	normalizeAndScale(buf, "kick-deep")
	globalVoiceCache.Put(key, buf)
	return &cVoice{buf: buf}
}

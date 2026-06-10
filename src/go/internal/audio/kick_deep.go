//go:build !test && !js

package audio

// KickDeep renders an 808-style deep sub kick. After the Phase-3 kick-family
// migration the legacy render_kick_deep C path is deleted; the no-edit voice
// renders through the modular engine via renderKickDeepVoice
// (kick_modular_native.go — the baked recipe-default ModularParams).
type KickDeep struct{}

// NewVoice generates a deep kick hit via the modular fast path.
func (KickDeep) NewVoice(bpm, sampleRate int) Voice {
	key := voiceCacheKey{instrumentID: "kick-deep", sampleRate: sampleRate}
	if buf, ok := globalVoiceCache.Get(key); ok {
		return &cVoice{buf: buf}
	}
	cfg := ConfigForInstrument("kick-deep")
	samples := int(float64(sampleRate) * cfg.DurationSec)
	buf := make([]float32, samples)
	renderKickDeepVoice(buf, sampleRate, samples)
	normalizeAndScale(buf, "kick-deep")
	globalVoiceCache.Put(key, buf)
	return &cVoice{buf: buf}
}

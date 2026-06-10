//go:build !test && !js

package audio

// Kick renders a bass drum. After the Phase-3 kick-family migration the legacy
// render_kick C path is deleted; the no-edit voice renders through the modular
// engine via renderKickVoice (kick_modular_native.go — the baked recipe-default
// ModularParams), keeping byte-identity with the recipe path.
type Kick struct{}

// NewVoice generates a kick hit via the modular fast path.
// Uses voice cache to avoid redundant renders for identical parameters.
func (Kick) NewVoice(bpm, sampleRate int) Voice {
	key := voiceCacheKey{instrumentID: "kick", sampleRate: sampleRate}
	if buf, ok := globalVoiceCache.Get(key); ok {
		return &cVoice{buf: buf}
	}
	cfg := ConfigForInstrument("kick")
	samples := int(float64(sampleRate) * cfg.DurationSec)
	buf := make([]float32, samples)
	renderKickVoice(buf, sampleRate, samples)
	normalizeAndScale(buf, "kick")
	globalVoiceCache.Put(key, buf)
	return &cVoice{buf: buf}
}

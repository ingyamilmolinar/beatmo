//go:build !test && !js

package audio

// Tom renders a pitched drum tone with a slight noise attack. After the Phase-4
// tom-family migration the legacy render_tom C path is deleted; the no-edit voice
// renders through the modular engine via renderTomVoice (tom_modular_native.go —
// the baked recipe-default ModularParams), keeping byte-identity with the recipe
// path.
type Tom struct{}

// NewVoice generates a tom hit via the modular fast path.
// Uses voice cache to avoid redundant renders for identical parameters.
func (Tom) NewVoice(bpm, sampleRate int) Voice {
	key := voiceCacheKey{instrumentID: "tom", sampleRate: sampleRate}
	if buf, ok := globalVoiceCache.Get(key); ok {
		return &cVoice{buf: buf}
	}
	cfg := ConfigForInstrument("tom")
	samples := int(float64(sampleRate) * cfg.DurationSec)
	buf := make([]float32, samples)
	renderTomVoice(buf, sampleRate, samples)
	normalizeAndScale(buf, "tom")
	globalVoiceCache.Put(key, buf)
	return &cVoice{buf: buf}
}

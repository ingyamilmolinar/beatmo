//go:build !test && !js

package audio

// Ride renders a bright metallic ride cymbal with bell-like tone.
type Ride struct{}

// NewVoice generates a ride cymbal hit via the C renderer.
func (Ride) NewVoice(bpm, sampleRate int) Voice {
	key := voiceCacheKey{instrumentID: "ride", sampleRate: sampleRate}
	if buf, ok := globalVoiceCache.Get(key); ok {
		return &cVoice{buf: buf}
	}
	cfg := ConfigForInstrument("ride")
	samples := int(float64(sampleRate) * cfg.DurationSec)
	buf := make([]float32, samples)
	renderRideVoice(buf, sampleRate, samples) // modular fast path (Phase-6 cutover)
	normalizeAndScale(buf, "ride")
	globalVoiceCache.Put(key, buf)
	return &cVoice{buf: buf}
}

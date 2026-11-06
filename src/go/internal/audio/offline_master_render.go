//go:build !test && !js

package audio

// Offline master-mix render against the LIVE engine configuration.
//
// RenderSongOffline (offline_song_render.go) is the *songrender* entry point:
// it OVERWRITES the per-instrument engine config from its SongRenderInstrument
// list before rendering, because its caller owns a template, not a live
// session. That makes it useless for verifying that a live session's audio
// survives an export/import round trip — it would install the very config the
// test is trying to prove was preserved.
//
// RenderMasterMixOffline schedules the same way but touches NO configuration.
// Whatever the engine currently holds — channel volume and pan, insert-effect
// chains, per-channel EQ, delay/reverb sends, master EQ, compressor and
// limiter — is what gets rendered. The returned PCM is the captured MASTER
// bus, post headroom/EQ/compressor/limiter and pre-int16, i.e. the same signal
// the audio device would receive.
//
// The render is a pure function of (engine config, notes, sampleRate):
// voices are seeded from their params (never from wall-clock or rand), and
// every pass starts with ResetDSPState so no filter delay line, LFO phase,
// compressor envelope or reverb tail survives from a preceding render. Two
// calls with the same inputs are therefore byte-identical, which is what makes
// an export/import audio-identity assertion meaningful.
func RenderMasterMixOffline(notes []SongRenderNote, sampleRate, totalSamples int) []float64 {
	return renderOnePass(notes, sampleRate, totalSamples, "")
}

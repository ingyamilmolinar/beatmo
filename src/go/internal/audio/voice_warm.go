package audio

// Voice pre-warming on instrument switch.
//
// When a row is switched to a different synth instrument mid-playback, the new
// instrument's per-pitch voice cache is cold, so the first notes pay a cold
// render (catastrophic in the browser: ~420ms for a 7-voice unison organ on
// emscripten's software libm; negligible ~0.3ms native). WarmInstrument
// pre-renders the likely pitches off the hot path so playback hits a warm
// cache. It is called from the UI's SetInstrument.
//
// The behaviour is kept identical across platforms to avoid drift: the same
// call warms the same cache. Only the mechanism differs — the browser renders
// off-thread on the render worker(s); desktop renders inline (native render is
// ~1400x faster, so off-threading buys nothing and would add concurrency
// surface). Each platform's warmInstrumentPlatform gates non-melodic
// instruments down to the base pitch, since only pitch-aware recipes keep a
// per-pitch cache.

// WarmInstrument is a package var (not a plain func) so UI tests can observe
// the call — SetInstrument invokes it at every instrument change. Production
// dispatches to the platform implementation.
var WarmInstrument = func(id string, pitches []int) {
	if id == "" || len(pitches) == 0 {
		return
	}
	warmInstrumentPlatform(id, pitches)
}

// DefaultWarmPitches is the candidate semitone spread SetInstrument passes for a
// switch: a modest range covering common melodic play. Non-melodic instruments
// are collapsed to the base pitch inside warmInstrumentPlatform, so passing the
// full spread for a drum is harmless (deduped to one render).
var DefaultWarmPitches = []int{-12, -7, -5, 0, 4, 7, 12}

package audio

import "github.com/ingyamilmolinar/beatmo/internal/instrument"

// A row switched mid-playback starts with a cold per-pitch cache, and a cold render is ~420 ms in the browser for a heavy
// voice. Same call, same cache on both platforms: the browser renders on its workers, desktop inline (fast enough that
// off-threading would only add concurrency).

// A var so UI tests can observe the call.
var WarmInstrument = func(id instrument.ID, pitches []int) {
	if id == "" || len(pitches) == 0 {
		return
	}
	warmInstrumentPlatform(id, pitches)
}

// Non-melodic instruments collapse to the base pitch, so the full spread for a drum is one render.
var DefaultWarmPitches = []int{-12, -7, -5, 0, 4, 7, 12}

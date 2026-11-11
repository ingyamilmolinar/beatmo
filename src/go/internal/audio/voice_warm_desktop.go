//go:build !test && !js

package audio

import "github.com/ingyamilmolinar/beatmo/internal/instrument"

// warmInstrumentPlatform (desktop) pre-renders id's voices into globalVoiceCache
// inline. Native render is ~0.3ms/voice, so a handful of pitches costs a few ms
// on the caller (UI) goroutine — imperceptible, and it avoids the concurrency
// surface of a background render racing the sequencer's renders. tryRecipeVoicePitched
// is a cache-miss render + Put (or a no-op cache hit), so this simply primes the
// cache; the returned voice is discarded.
func warmInstrumentPlatform(id instrument.ID, pitches []int) {
	sr := SampleRate()
	// Only pitch-aware (melodic) recipes keep a per-pitch cache; everything else
	// (drums, single-key FM) shares one bare key, so warm just the base pitch.
	if !docMelodic(id) {
		tryRecipeVoiceOptsOrigin(id, sr, false, originPrewarm)
		return
	}
	seen := make(map[int]bool, len(pitches))
	for _, p := range pitches {
		if seen[p] {
			continue
		}
		seen[p] = true
		tryRecipeVoicePitchedOrigin(id, sr, float64(p), originPrewarm)
	}
}

//go:build !test && !js

package audio

// warmInstrumentPlatform (desktop) pre-renders id's voices into globalVoiceCache
// inline. Native render is ~0.3ms/voice, so a handful of pitches costs a few ms
// on the caller (UI) goroutine — imperceptible, and it avoids the concurrency
// surface of a background render racing the sequencer's renders. tryRecipeVoicePitched
// is a cache-miss render + Put (or a no-op cache hit), so this simply primes the
// cache; the returned voice is discarded.
func warmInstrumentPlatform(id string, pitches []int) {
	sr := SampleRate()
	b := currentBPMForWarm()
	// Only pitch-aware (melodic) recipes keep a per-pitch cache; everything else
	// (drums, single-key FM) shares one bare key, so warm just the base pitch.
	if !pitchAwareRecipe(RecipeForInstrument(id)) {
		tryRecipeVoicePitched(id, b, sr, 0)
		return
	}
	seen := make(map[int]bool, len(pitches))
	for _, p := range pitches {
		if seen[p] {
			continue
		}
		seen[p] = true
		tryRecipeVoicePitched(id, b, sr, float64(p))
	}
}

// currentBPMForWarm mirrors the bpm sample_capture uses so warmed cache keys
// match the ones the sequencer will look up at play time.
func currentBPMForWarm() int {
	b := bpm
	if b <= 0 {
		b = 120
	}
	return b
}

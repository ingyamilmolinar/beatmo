//go:build !test && !js

package audio

import "github.com/ingyamilmolinar/beatmo/internal/instrument"

// Inline: a few native renders cost milliseconds on the UI goroutine and avoid a background render racing the
// sequencer's.
func warmInstrumentPlatform(id instrument.ID, pitches []int) {
	sr := SampleRate()
	// Only melodic voices keep a per-pitch cache.
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

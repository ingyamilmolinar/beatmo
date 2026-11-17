//go:build !test && !js

package audio

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// A sample-edit must rotate the binding, NOT blanket-drop every other pitch's
// cached voice. hashSampleEdit is already XORed into paramsHash, so the key
// rotates by itself; an additional voiceCacheInvalidate is redundant and
// recreates the 93%-miss knob-drag storm that SetInstrumentParam was fixed to
// avoid (synth_recipe.go:256-267).
func TestSetSampleEditDoesNotBlanketInvalidateOtherPitches(t *testing.T) {
	const id = roster.OrganChurch
	globalVoiceCache.Clear()
	ClearSampleEdit(id)
	ResetInstrumentParams(id)
	t.Cleanup(func() { ClearSampleEdit(id); ResetInstrumentParams(id) })

	// Warm three distinct pitches.
	pitches := []float64{-2, 0, 5}
	for _, p := range pitches {
		PrewarmVoice(id, p)
	}
	for _, p := range pitches {
		if !VoiceCached(id, p) {
			t.Fatalf("setup: pitch %v did not warm", p)
		}
	}

	// A sample edit rotates the binding for EVERY pitch (the edit hash is in
	// the key), so VoiceCached for the NEW binding is legitimately false.
	// What must NOT happen is the OLD entries being dropped: they are what a
	// pre-warmed neighbour relies on, and dropping them makes every subsequent
	// note a cold render.
	before := voiceCacheEntryCountForTest()
	SetSampleEdit(id, SampleEdit{StartFrac: 0, EndFrac: 0.9, GainDB: -3})
	after := voiceCacheEntryCountForTest()

	if after < before {
		t.Errorf("SetSampleEdit dropped %d cached voice entries (%d -> %d). "+
			"The edit hash already rotates the key; the blanket invalidate is "+
			"redundant and re-creates the knob-drag storm.", before-after, before, after)
	}
}

// voiceCacheEntryCountForTest reports the number of live cache entries.
func voiceCacheEntryCountForTest() int {
	globalVoiceCache.mu.RLock()
	defer globalVoiceCache.mu.RUnlock()
	return len(globalVoiceCache.entries)
}

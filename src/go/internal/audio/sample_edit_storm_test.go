//go:build !test && !js

package audio

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// The hash already rotates the key; an extra invalidation would recreate the knob-drag miss storm.
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

	// The NEW binding is legitimately uncached; the OLD entries must survive, since pre-warmed neighbours rely on them.
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

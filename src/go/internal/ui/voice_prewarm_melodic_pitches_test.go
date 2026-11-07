package ui

import (
	"sync"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// A melodic voice is rendered AT its pitch, so a node pitch left out cold-renders on dispatch.
func TestMelodicRowNodePitchesWarmed(t *testing.T) {
	pitches := []float64{0, 5, -2, 7}
	g := newMelodicCircuitGame(t, pitches...)

	var mu sync.Mutex
	warmed := map[warmKey]struct{}{}
	g.voicePrewarmer.warmFn = func(id instrument.ID, pitch float64) {
		mu.Lock()
		warmed[warmKey{id: id, pitch: pitch}] = struct{}{}
		mu.Unlock()
	}

	// warmPitch skips cached pitches and the cache is process-global, so start cold: this measures enumeration, not
	// history.
	audio.ClearVoiceCacheForTest()
	g.updateBeatInfos()

	mu.Lock()
	defer mu.Unlock()
	for _, p := range pitches {
		if _, ok := warmed[warmKey{id: testMelodicInst, pitch: p}]; !ok {
			t.Errorf("%s pitch %+.0f was NOT enumerated for pre-warm (warmed %v)", testMelodicInst, p, warmed)
		}
	}
}

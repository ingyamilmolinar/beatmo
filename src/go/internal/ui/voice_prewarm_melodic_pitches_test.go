package ui

import (
	"sync"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// TestMelodicRowNodePitchesWarmed: computing a circuit's paths submits every
// node pitch of a melodic row to the prewarmer. A melodic voice is rendered AT
// its pitch, so a pitch left out cold-renders on dispatch — the crash logs that
// started this showed exactly that for a melodic row's +5 and −2 nodes.
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

	// warmPitch submits only pitches that are NOT already cached, and the voice
	// cache is process-global (building the circuit, or an earlier test, may
	// have warmed it). Start cold so this measures enumeration, not history.
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

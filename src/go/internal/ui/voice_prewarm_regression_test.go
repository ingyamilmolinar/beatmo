package ui

import (
	"sync"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// TestOverduePanicFixed_OrganAtMinus2WarmedBeforeDispatch is the functional
// proof for the overdue/absmax crash: a circuit whose organ row fires a node at
// pitch -2 (the exact crash pitch) must have that voice WARMED by the time the
// sequencer would dispatch it — so dispatch is a cache hit and no beat lands
// overdue. Drives the real graph->updateBeatInfos->enumeration path.
func TestOverduePanicFixed_OrganAtMinus2WarmedBeforeDispatch(t *testing.T) {
	g := newTestGameWithOrganAtPitch(t, roster.OrganChurch, -2)

	var mu sync.Mutex
	warmed := map[warmKey]struct{}{}
	g.voicePrewarmer.warmFn = func(id instrument.ID, pitch float64) {
		mu.Lock()
		warmed[warmKey{id: id, pitch: pitch}] = struct{}{}
		mu.Unlock()
	}

	g.updateBeatInfos()

	mu.Lock()
	_, ok := warmed[warmKey{id: roster.OrganChurch, pitch: -2.0}]
	mu.Unlock()
	if !ok {
		t.Fatalf("crash pitch -2 was not pre-warmed before dispatch; got %v", warmed)
	}
}

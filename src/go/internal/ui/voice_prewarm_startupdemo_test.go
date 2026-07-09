package ui

import (
	"sync"
	"testing"

	assets_pkg "github.com/ingyamilmolinar/beatmo/internal/assets"
)

// TestStartupDemoOrganPitchesWarmed reproduces the real startup-demo load path
// (g.Import(StartupDemoJSON) -> updateBeatInfos) and records every (id, pitch)
// the prewarmer submits. It asserts the organ-church node pitches that the
// crash logs showed cold-rendering (+5, -2) are enumerated — deterministic
// evidence for why dispatch still hit a cold render.
func TestStartupDemoOrganPitchesWarmed(t *testing.T) {
	g := buildTestGame(t)

	var mu sync.Mutex
	warmed := map[warmKey]struct{}{}
	g.voicePrewarmer.warmFn = func(id string, pitch float64) {
		mu.Lock()
		warmed[warmKey{id: id, pitch: pitch}] = struct{}{}
		mu.Unlock()
	}

	if err := g.Import(assets_pkg.StartupDemoJSON); err != nil {
		t.Fatalf("import startup demo: %v", err)
	}
	g.updateBeatInfos()

	mu.Lock()
	defer mu.Unlock()
	t.Logf("warmed pairs (%d):", len(warmed))
	for k := range warmed {
		t.Logf("  id=%s pitch=%.2f", k.id, k.pitch)
	}
	for _, p := range []float64{5.0, -2.0} {
		if _, ok := warmed[warmKey{id: "organ-church", pitch: p}]; !ok {
			t.Errorf("organ-church pitch %.1f was NOT enumerated for pre-warm", p)
		}
	}
}

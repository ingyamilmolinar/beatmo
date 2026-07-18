package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// A param edit must warm the NEW binding at every pitch the instrument is
// currently enumerated to play, so the next trigger hits the cache instead of
// cold-rendering on the audio dispatch goroutine.
func TestParamEditWarmsNewBindingAtEnumeratedPitches(t *testing.T) {
	g := buildTestGame(t)
	p := g.voicePrewarmer
	if p == nil {
		t.Fatal("game has no voicePrewarmer")
	}

	type warm struct {
		id    string
		pitch float64
	}
	var got []warm
	p.mu.Lock()
	p.pitchesByInst = map[string][]float64{"organ-church": {-2, 0, 5}}
	p.mu.Unlock()
	p.warmParamsFn = func(id string, pitch float64, params audio.RecipeParams) {
		got = append(got, warm{id, pitch})
	}

	p.warmBinding("organ-church", audio.RecipeParams{"drive": 0.5})

	if len(got) != 3 {
		t.Fatalf("warmBinding warmed %d pitches, want 3 (the enumerated set): %+v", len(got), got)
	}
	for _, w := range got {
		if w.id != "organ-church" {
			t.Fatalf("warmed the wrong instrument: %q", w.id)
		}
	}
}

// An edit to an instrument with no enumerated pitches must not warm anything
// (nothing is going to trigger it).
func TestParamEditOnUnenumeratedInstrumentWarmsNothing(t *testing.T) {
	g := buildTestGame(t)
	p := g.voicePrewarmer
	called := 0
	p.warmParamsFn = func(id string, pitch float64, params audio.RecipeParams) { called++ }
	p.warmBinding("not-in-the-graph", audio.RecipeParams{"drive": 0.5})
	if called != 0 {
		t.Fatalf("warmed %d voices for an instrument that never triggers", called)
	}
}

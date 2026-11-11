package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// Every dial names the graph view that explains it and the words its two ends
// are labelled with (spec 2026-10-24 §4.5) — data, checked here against the
// view registry and the translation catalogue.
func TestEveryDialNamesAViewAndEndWords(t *testing.T) {
	f, err := loadSynthDials()
	if err != nil {
		t.Fatal(err)
	}
	keys := map[i18n.Key]bool{}
	for _, k := range i18n.AllKeys() {
		keys[k] = true
	}
	for _, d := range f.Dials {
		if _, ok := synthViewByName(d.View, audio.RecipeParams{}); !ok {
			t.Errorf("dial %q names view %q, which the graph does not draw", d.ID, d.View)
		}
		for _, k := range []string{d.Lo, d.Hi} {
			if !keys[i18n.Key(k)] {
				t.Errorf("dial %q end word %q is not in the i18n catalogue", d.ID, k)
			}
		}
	}
}

// "lfo-target" follows where the LFO is routed.
func TestLFOTargetViewFollowsTheRouting(t *testing.T) {
	for target, want := range map[float64]synthView{0: viewLoudness, 1: viewPitch, 2: viewColourTime} {
		if v, _ := synthViewByName("lfo-target", audio.RecipeParams{"lfo_target": target}); v != want {
			t.Errorf("lfo_target %v -> %v, want %v", target, v, want)
		}
	}
}

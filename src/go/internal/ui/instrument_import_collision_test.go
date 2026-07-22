//go:build test

package ui

import (
	"encoding/json"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

func TestImportCollisionConfigMapping(t *testing.T) {
	got := CollisionChoice(-1)
	cfg := importCollisionConfig(func(c CollisionChoice) { got = c })
	if cfg.confirmKey != i18n.KeyCapReplace || cfg.cancelKey != i18n.KeyCapKeepBoth {
		t.Errorf("labels: confirm=%q cancel=%q", cfg.confirmKey, cfg.cancelKey)
	}
	if !cfg.destructive {
		t.Error("Replace should be styled destructive")
	}
	cfg.onConfirm()
	if got != CollisionReplace {
		t.Errorf("confirm → %v, want Replace", got)
	}
	cfg.onCancel()
	if got != CollisionKeepBoth {
		t.Errorf("cancel → %v, want KeepBoth", got)
	}
}

func TestBundleUserCollisionsDetection(t *testing.T) {
	// A builtin recipe id never collides (import re-namespaces).
	b1 := &audio.InstrumentBundle{Instruments: []audio.InstrumentDoc{{Recipe: audio.RecipeDoc{ID: "drum-snare"}}}}
	if got := bundleUserCollisions(b1); len(got) != 0 {
		t.Errorf("builtin flagged as collision: %+v", got)
	}
	// A registered user recipe id collides.
	if _, err := audio.RegisterUserRecipeFromBase("user.coll.test", "X", "drum-snare", nil); err != nil {
		t.Fatalf("register: %v", err)
	}
	t.Cleanup(func() { audio.UnregisterRecipeForTest("user.coll.test") })
	b2 := &audio.InstrumentBundle{Instruments: []audio.InstrumentDoc{{Recipe: audio.RecipeDoc{ID: "user.coll.test"}}}}
	if got := bundleUserCollisions(b2); len(got) != 1 {
		t.Errorf("user collision not detected: %+v", got)
	}
	// A fresh id doesn't collide.
	b3 := &audio.InstrumentBundle{Instruments: []audio.InstrumentDoc{{Recipe: audio.RecipeDoc{ID: "user.brand.new"}}}}
	if got := bundleUserCollisions(b3); len(got) != 0 {
		t.Errorf("fresh id flagged: %+v", got)
	}
}

func TestImportDefersToDialogOnCollision(t *testing.T) {
	g := newInstrumentTestGame(t)
	dv := g.drum
	dv.selRow = 0

	if _, err := audio.RegisterUserRecipeFromBase("user.defer.test", "Deferred", "drum-snare", nil); err != nil {
		t.Fatalf("register: %v", err)
	}
	t.Cleanup(func() { audio.UnregisterRecipeForTest("user.defer.test") })

	bundle := audio.InstrumentBundle{
		Format: audio.InstrumentBundleFormat, Version: audio.InstrumentBundleVersion,
		Instruments: []audio.InstrumentDoc{{Name: "Deferred", Kind: "synth", Recipe: audio.RecipeDoc{ID: "user.defer.test", BaseRecipe: "drum-snare"}}},
	}
	raw, _ := json.Marshal(bundle)
	prev := selectJSONAsyncFn
	selectJSONAsyncFn = func(cb func([]byte, string, error)) { cb(raw, "x.beatmo-inst.json", nil) }
	t.Cleanup(func() { selectJSONAsyncFn = prev })

	dv.importInstrumentFromFile()
	dv.drainInstrumentImport()

	// Colliding import must DEFER to the dialog, not apply silently.
	if dv.portal() == nil || !dv.portal().Has("instrument-import-collision") {
		t.Fatal("collision did not open the Replace/Keep-both dialog")
	}
}

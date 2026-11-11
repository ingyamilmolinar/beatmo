//go:build test

package ui

import (
	"encoding/json"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"

	"github.com/ingyamilmolinar/beatmo/internal/schema"
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
	withPrivateInstrumentLibrary(t)
	snare, _ := audio.ShippedDoc(roster.Snare)
	withID := func(id instrument.ID) *instrument.Doc { d := snare.Clone(); d.ID = id; return &d }

	// A shipped id never collides (import re-namespaces).
	b1 := &audio.InstrumentBundle{Instruments: []audio.BundleInstrument{{Synth: withID(roster.Snare)}}}
	if got := bundleUserCollisions(b1); len(got) != 0 {
		t.Errorf("shipped flagged as collision: %+v", got)
	}
	// A saved user instrument id collides.
	if err := audio.SaveLibraryDoc(*withID("user.coll.test")); err != nil {
		t.Fatalf("save: %v", err)
	}
	b2 := &audio.InstrumentBundle{Instruments: []audio.BundleInstrument{{Synth: withID("user.coll.test")}}}
	if got := bundleUserCollisions(b2); len(got) != 1 {
		t.Errorf("user collision not detected: %+v", got)
	}
	// A fresh id doesn't collide.
	b3 := &audio.InstrumentBundle{Instruments: []audio.BundleInstrument{{Synth: withID("user.brand.new")}}}
	if got := bundleUserCollisions(b3); len(got) != 0 {
		t.Errorf("fresh id flagged: %+v", got)
	}
}

func TestImportDefersToDialogOnCollision(t *testing.T) {
	g := newDocsGame(t, roster.Snare)
	dv := g.drum
	dv.selRow = 0

	snare, _ := audio.ShippedDoc(roster.Snare)
	saved := snare.Clone()
	saved.ID, saved.Name = "user.defer.test", "Deferred"
	if err := audio.SaveLibraryDoc(saved); err != nil {
		t.Fatalf("save: %v", err)
	}

	bundle := audio.InstrumentBundle{
		Envelope: schema.Current(), Format: audio.InstrumentBundleFormat,
		Instruments: []audio.BundleInstrument{{Name: "Deferred", Kind: audio.BundleSynth, Synth: &saved}},
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

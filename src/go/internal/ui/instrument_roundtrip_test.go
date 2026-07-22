//go:build test

package ui

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

func newInstrumentTestGame(t *testing.T) *Game {
	t.Helper()
	assertDefaultParityState(t)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	return g
}

// TestInstrumentBundleRoundTripSynth exports a customized synth instrument and
// re-imports it: the tone (base+seed), normalized effects, and EQ survive; the
// builtin is never clobbered (import re-namespaces).
func TestInstrumentBundleRoundTripSynth(t *testing.T) {
	g := newInstrumentTestGame(t)
	const inst, base = "snare", "drum-snare"
	reg := audio.RecipeRegistrations()[base]
	if reg == nil || len(reg.Params) == 0 {
		t.Skipf("%s unavailable", base)
	}
	g.drum.Rows[0].Instrument = inst
	audio.BindInstrumentToRecipe(inst, base)
	t.Cleanup(func() { audio.ResetInstrumentParams(inst); audio.SetInsertEffects(inst, nil) })

	p := reg.Params[0]
	shipped := audio.RecipeShippedDefaults(base)
	newVal := p.Min
	if math.Abs(newVal-shipped[p.Name]) < 1e-9 {
		newVal = p.Max
	}
	audio.SetInstrumentParam(inst, p.Name, newVal)
	audio.SetInsertEffects(inst, []audio.EffectSlot{{Type: "bitcrusher", Enabled: true, Params: map[string]float64{"bits": 6}}})
	g.drum.Rows[0].EQGainsDB = make([]float64, 10)
	g.drum.Rows[0].EQGainsDB[2] = 4.0

	bundle, err := g.drum.BuildInstrumentBundle(0)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	d := bundle.Instruments[0]
	if d.Recipe.BaseRecipe != base {
		t.Errorf("base recipe = %q, want %q", d.Recipe.BaseRecipe, base)
	}
	if len(bundle.Effects) != 1 || len(d.EffectChain) != 1 {
		t.Errorf("effects not normalized: defs=%d chain=%v", len(bundle.Effects), d.EffectChain)
	}
	if d.EQ == nil || len(d.EQ.GainsDB) < 3 || d.EQ.GainsDB[2] != 4.0 {
		t.Errorf("eq not captured: %+v", d.EQ)
	}

	results, err := g.drum.ApplyInstrumentBundle(&bundle, func(ImportCollision) CollisionChoice { return CollisionKeepBoth }, -1, nil)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("want 1 result, got %d", len(results))
	}
	newID := results[0].ID
	t.Cleanup(func() { audio.UnregisterRecipeForTest(newID) })

	if newID == base || audio.RecipeIsBuiltin(newID) {
		t.Fatalf("import clobbered/aliased the builtin: newID=%q", newID)
	}
	if !audio.RecipeIsBuiltin(base) {
		t.Errorf("builtin %q no longer builtin after import", base)
	}

	// Effective tone matches.
	srcEff := audio.MergeRecipeDefaults(base, audio.GetInstrumentParams(inst))
	impEff := audio.MergeRecipeDefaults(newID, audio.GetInstrumentParams(newID))
	if math.Abs(impEff[p.Name]-srcEff[p.Name]) > 1e-9 {
		t.Errorf("param %q: imported %v, source %v", p.Name, impEff[p.Name], srcEff[p.Name])
	}
	// Effects reconstructed.
	fx := audio.GetInsertEffects(newID)
	if len(fx) != 1 || fx[0].Type != "bitcrusher" || fx[0].Params["bits"] != 6 {
		t.Errorf("effects not reconstructed: %+v", fx)
	}
}

// TestInstrumentBundleWavReferenceRoundTrip exports a user-WAV instrument (a
// reference, NO PCM in the file) and re-imports it, resolving the WAV via a fake
// locator that verifies the content hash.
func TestInstrumentBundleWavReferenceRoundTrip(t *testing.T) {
	g := newInstrumentTestGame(t)
	const inst, base = "wavinst", "drum-snare"
	if audio.RecipeRegistrations()[base] == nil {
		t.Skipf("%s unavailable", base)
	}
	g.drum.Rows[0].Instrument = inst
	audio.BindInstrumentToRecipe(inst, base)
	pcm := []float32{0.1, -0.2, 0.3, -0.4}
	audio.PutUserSample(inst, pcm, 48000)
	t.Cleanup(func() { audio.ResetInstrumentParams(inst) })

	bundle, err := g.drum.BuildInstrumentBundle(0)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	d := bundle.Instruments[0]
	if d.Kind != "wav" {
		t.Errorf("kind = %q, want wav", d.Kind)
	}
	if d.WavRef == nil || d.WavRef.Sha256 == "" || len(d.WavRef.Sources) == 0 {
		t.Fatalf("wav ref not built: %+v", d.WavRef)
	}
	// The file itself carries NO PCM — only a reference.
	if len(d.WavRef.Name) == 0 {
		t.Error("wav ref missing name")
	}

	// Import via the REAL localstorage locator (re-encodes the source sample's
	// PCM to the canonical WAV bytes → hash matches the pin).
	results, err := g.drum.ApplyInstrumentBundle(&bundle, func(ImportCollision) CollisionChoice { return CollisionKeepBoth }, -1, g.drum.wavImportLocators())
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	newID := results[0].ID
	t.Cleanup(func() { audio.UnregisterRecipeForTest(newID) })

	if results[0].WavState != audio.WavResolveMatched {
		t.Errorf("wav state = %v, want Matched", results[0].WavState)
	}
	if !audio.IsUserSample(newID) {
		t.Errorf("imported WAV instrument not registered as a user sample")
	}

	// A mismatching locator (different bytes) reports a mismatch, not a bind.
	badLoc := map[string]audio.WavLocator{
		audio.WavSourceLocalStorage: func(string) ([]byte, bool) { return []byte("garbage-not-the-pinned-wav"), true },
	}
	res2, _ := g.drum.ApplyInstrumentBundle(&bundle, func(ImportCollision) CollisionChoice { return CollisionKeepBoth }, -1, badLoc)
	t.Cleanup(func() {
		if len(res2) > 0 {
			audio.UnregisterRecipeForTest(res2[0].ID)
		}
	})
	if len(res2) == 0 || res2[0].WavState != audio.WavResolveMismatch {
		t.Errorf("expected mismatch, got %+v", res2)
	}
}

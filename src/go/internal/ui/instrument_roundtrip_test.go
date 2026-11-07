//go:build test

package ui

import (
	"math"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
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

// Import re-namespaces, so the builtin is never clobbered.
func TestInstrumentBundleRoundTripSynth(t *testing.T) {
	g := newDocsGame(t, roster.Snare)
	const inst = roster.Snare
	t.Cleanup(func() { audio.SetInsertEffects(inst, nil) })

	audio.SetInstrumentParam(inst, "gen1_snare_tune", 1.2)
	audio.SetInsertEffects(inst, []audio.EffectSlot{{Type: "bitcrusher", Enabled: true, Params: map[string]float64{"bits": 6}}})
	rowEQ(g.drum.Rows[0]).GainsDB = make([]float64, 10)
	rowEQ(g.drum.Rows[0]).GainsDB[2] = 4.0

	bundle, err := g.drum.BuildInstrumentBundle(0)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	d := bundle.Instruments[0]
	if d.Synth == nil || d.Synth.ID != inst {
		t.Fatalf("bundle must carry the instrument document: %+v", d.Synth)
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
	if newID == inst || !strings.HasPrefix(string(newID), "user.") {
		t.Fatalf("import clobbered/aliased the shipped instrument: newID=%q", newID)
	}
	if lib, ok := audio.LibraryDoc(inst); !ok || lib.Origin != "" {
		t.Errorf("shipped %q lost from the library after import", inst)
	}
	if _, ok := audio.UserLibraryDoc(newID); !ok {
		t.Errorf("imported instrument %q not saved to the user library", newID)
	}

	// Effective tone matches.
	if got, want := audio.GetInstrumentParams(newID)["gen1_snare_tune"], audio.GetInstrumentParams(inst)["gen1_snare_tune"]; math.Abs(got-want) > 1e-9 || want != 1.2 {
		t.Errorf("gen1_snare_tune: imported %v, source %v", got, want)
	}
	// Effects reconstructed.
	fx := audio.GetInsertEffects(newID)
	if len(fx) != 1 || fx[0].Type != "bitcrusher" || fx[0].Params["bits"] != 6 {
		t.Errorf("effects not reconstructed: %+v", fx)
	}
}

// The file carries a reference, never PCM; the fake locator verifies the content hash.
func TestInstrumentBundleWavReferenceRoundTrip(t *testing.T) {
	g := newDocsGame(t, roster.Snare)
	const inst = roster.Snare
	pcm := []float32{0.1, -0.2, 0.3, -0.4}
	audio.PutUserSample(inst, pcm, 48000)
	t.Cleanup(func() { audio.DropBuiltinSampleOverride(inst) })

	bundle, err := g.drum.BuildInstrumentBundle(0)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	d := bundle.Instruments[0]
	if d.Kind != audio.BundleWAV {
		t.Errorf("kind = %q, want wav", d.Kind)
	}
	if d.WavRef == nil || d.WavRef.Sha256 == "" || len(d.WavRef.Sources) == 0 {
		t.Fatalf("wav ref not built: %+v", d.WavRef)
	}
	// The file itself carries NO PCM — only a reference.
	if len(d.WavRef.Name) == 0 {
		t.Error("wav ref missing name")
	}

	// The real localstorage locator re-encodes the sample's PCM to the canonical WAV bytes, so the hash matches.
	results, err := g.drum.ApplyInstrumentBundle(&bundle, func(ImportCollision) CollisionChoice { return CollisionKeepBoth }, -1, g.drum.wavImportLocators())
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	newID := results[0].ID
	t.Cleanup(func() { audio.UnregisterSamplePCM(newID) })

	if results[0].WavState != audio.WavResolveMatched {
		t.Errorf("wav state = %v, want Matched", results[0].WavState)
	}
	if !audio.IsUserSample(newID) {
		t.Errorf("imported WAV instrument not registered as a user sample")
	}

	// A mismatching locator (different bytes) reports a mismatch, not a bind.
	badLoc := map[audio.WavSourceKind]audio.WavLocator{
		audio.WavSourceLocalStorage: func(string) ([]byte, bool) { return []byte("garbage-not-the-pinned-wav"), true },
	}
	res2, _ := g.drum.ApplyInstrumentBundle(&bundle, func(ImportCollision) CollisionChoice { return CollisionKeepBoth }, -1, badLoc)
	t.Cleanup(func() {
		if len(res2) > 0 {
			audio.UnregisterSamplePCM(res2[0].ID)
		}
	})
	if len(res2) == 0 || res2[0].WavState != audio.WavResolveMismatch {
		t.Errorf("expected mismatch, got %+v", res2)
	}
}

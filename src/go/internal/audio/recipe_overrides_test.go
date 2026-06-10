package audio

import (
	"encoding/json"
	"math"
	"sync/atomic"
	"testing"
)

// stubRecipeSource is the minimum the audio package needs from a
// persistence backend to drive ApplyUserRecipeOverrides. Tests build
// these directly; production callers pass userprefs.RecipeStore which
// satisfies the same shape.
type stubRecipeSource struct {
	overrides map[string]map[string]float64
	recipes   map[string][]byte
}

func (s *stubRecipeSource) LoadRecipeOverrides() (map[string]map[string]float64, error) {
	return s.overrides, nil
}

func (s *stubRecipeSource) LoadUserRecipes() (map[string][]byte, error) {
	return s.recipes, nil
}

// withGateOff restores the parity gate to its prior state on cleanup.
// Used after every test that flips it so subsequent tests run with the
// gate off (matches the production-default discipline).
func withGate(t *testing.T, on bool) {
	t.Helper()
	prev := SwapUseUserRecipeOverridesForTest(on)
	t.Cleanup(func() { SwapUseUserRecipeOverridesForTest(prev) })
}

// TestParityGateOffByDefault — the gate must default to false so the
// first time anyone calls Apply with a populated source nothing happens.
// This is the parity protection: test binaries never have to remember to
// stub userprefs.
func TestParityGateOffByDefault(t *testing.T) {
	if UseUserRecipeOverrides() {
		t.Fatalf("UseUserRecipeOverrides() = true at package load; want false (parity gate must default off)")
	}
}

// TestApplyWithGateOff_DoesNotMutateRegistry — even with a populated
// source, Apply must leave the registry untouched when the gate is off.
func TestApplyWithGateOff_DoesNotMutateRegistry(t *testing.T) {
	withGate(t, false)
	beforeHash := hashRecipeParams(RecipeDefaultParams("drum-snare"))

	src := &stubRecipeSource{
		overrides: map[string]map[string]float64{
			"drum-snare": {"decay": 1.7},
		},
	}
	if err := ApplyUserRecipeOverrides(src); err != nil {
		t.Fatalf("ApplyUserRecipeOverrides: %v", err)
	}
	afterHash := hashRecipeParams(RecipeDefaultParams("drum-snare"))
	if beforeHash != afterHash {
		t.Fatalf("registry defaults changed with gate off: before=%d after=%d", beforeHash, afterHash)
	}
}

// TestApplyWithGateOff_ModularRecipeNotMutated — the parity guarantee for
// the unified modular voice: with UseUserRecipeOverrides off, a populated
// override targeting synth-modular must leave the modular recipe's
// registered defaults (and every other recipe's) byte-identical. This is
// what keeps the existing golden suite untouched after adding the recipe.
//
// The override value is asserted to differ from the shipped default so the
// test is non-tautological: a pass means the gate genuinely blocked a real
// change, not that the override happened to be a no-op.
func TestApplyWithGateOff_ModularRecipeNotMutated(t *testing.T) {
	withGate(t, false)

	const overrideCutoff = 2000.0
	shipped := RecipeDefaultParams("synth-modular")
	if shipped["filter_cutoff"] == overrideCutoff {
		t.Fatalf("test fixture invalid: override %v equals shipped default; pick a different value", overrideCutoff)
	}
	beforeModular := hashRecipeParams(shipped)
	beforeSnare := hashRecipeParams(RecipeDefaultParams("drum-snare"))
	beforeBell := hashRecipeParams(RecipeDefaultParams("fm-bell"))

	src := &stubRecipeSource{
		overrides: map[string]map[string]float64{
			"synth-modular": {"filter_cutoff": overrideCutoff, "osc_type": 2},
		},
	}
	if err := ApplyUserRecipeOverrides(src); err != nil {
		t.Fatalf("ApplyUserRecipeOverrides: %v", err)
	}

	if got := hashRecipeParams(RecipeDefaultParams("synth-modular")); got != beforeModular {
		t.Fatalf("synth-modular defaults changed with gate off: before=%d after=%d", beforeModular, got)
	}
	if got := hashRecipeParams(RecipeDefaultParams("drum-snare")); got != beforeSnare {
		t.Fatalf("drum-snare defaults drifted after modular override with gate off: before=%d after=%d", beforeSnare, got)
	}
	if got := hashRecipeParams(RecipeDefaultParams("fm-bell")); got != beforeBell {
		t.Fatalf("fm-bell defaults drifted after modular override with gate off: before=%d after=%d", beforeBell, got)
	}
}

// TestApplyWithGateOn_MutatesRegistryAndInvalidatesCache — with the gate
// on, Apply must mutate the registered ParamDef defaults AND invalidate
// the voice cache for every instrument bound to the affected recipe.
//
// Captures: defaults change shape and value, voiceCacheInvalidate fires
// once per bound instrument, untouched recipes are unaffected.
func TestApplyWithGateOn_MutatesRegistryAndInvalidatesCache(t *testing.T) {
	// Restore the original default on cleanup so other tests see the
	// shipped value. Recompute on cleanup from a stub override of the
	// same shape so the global state is fully restored.
	originalDefault := -1.0
	for _, p := range RecipeRegistrations()["drum-snare"].Params {
		if p.Name == "decay" {
			originalDefault = p.Default
		}
	}
	if originalDefault < 0 {
		t.Fatalf("drum-snare has no decay param; cannot run test")
	}
	t.Cleanup(func() {
		SwapUseUserRecipeOverridesForTest(true)
		_ = ApplyUserRecipeOverrides(&stubRecipeSource{
			overrides: map[string]map[string]float64{
				"drum-snare": {"decay": originalDefault},
			},
		})
		SwapUseUserRecipeOverridesForTest(false)
	})

	// Hook the cache invalidator so we can count fires.
	var invalidations atomic.Int64
	prevInvalidator := voiceCacheInvalidate
	voiceCacheInvalidate = func(instID string) { invalidations.Add(1) }
	t.Cleanup(func() { voiceCacheInvalidate = prevInvalidator })

	withGate(t, true)
	src := &stubRecipeSource{
		overrides: map[string]map[string]float64{
			"drum-snare": {"decay": 1.7},
		},
	}
	if err := ApplyUserRecipeOverrides(src); err != nil {
		t.Fatalf("ApplyUserRecipeOverrides: %v", err)
	}
	// Registered default must reflect the override.
	got := RecipeDefaultParams("drum-snare")["decay"]
	if got != 1.7 {
		t.Errorf("drum-snare decay default = %v, want 1.7", got)
	}
	// At least one instrument bound to drum-snare must have been invalidated.
	// builtinInstrumentRecipeBindings binds "snare" → drum-snare and
	// "snare-2" → drum-snare, so >= 2 fires expected.
	if invalidations.Load() < 1 {
		t.Errorf("voiceCacheInvalidate fired %d times; expected >= 1", invalidations.Load())
	}
	// Unrelated recipe defaults stay intact.
	if RecipeDefaultParams("drum-kick")["decay"] == 1.7 {
		t.Errorf("unrelated recipe drum-kick was mutated")
	}
}

// TestApplyClampsAndDropsBadValues — out-of-range overrides clamp to
// [Min,Max]; NaN/Inf are silently dropped (sanitisation boundary
// duplicated from userprefs as documented in recipe_overrides.go).
func TestApplyClampsAndDropsBadValues(t *testing.T) {
	originalDecay := -1.0
	for _, p := range RecipeRegistrations()["drum-snare"].Params {
		if p.Name == "decay" {
			originalDecay = p.Default
		}
	}
	t.Cleanup(func() {
		SwapUseUserRecipeOverridesForTest(true)
		_ = ApplyUserRecipeOverrides(&stubRecipeSource{
			overrides: map[string]map[string]float64{"drum-snare": {"decay": originalDecay}},
		})
		SwapUseUserRecipeOverridesForTest(false)
	})

	withGate(t, true)
	src := &stubRecipeSource{
		overrides: map[string]map[string]float64{
			"drum-snare": {
				"decay": 999, // clamps to Max
				"tone":  math.NaN(),
				"drive": math.Inf(1),
			},
		},
	}
	if err := ApplyUserRecipeOverrides(src); err != nil {
		t.Fatalf("ApplyUserRecipeOverrides: %v", err)
	}
	defs := RecipeDefaultParams("drum-snare")
	// decay clamps to its Max (per GenericSynthParamDefs, Max=4).
	if defs["decay"] != 4 {
		t.Errorf("decay clamp: got %v want 4", defs["decay"])
	}
	// tone/drive NaN/Inf dropped — defaults retain their identity values
	// (tone identity = 0, drive identity = 0).
	if defs["tone"] != 0 {
		t.Errorf("tone after NaN reject: got %v want 0 (identity)", defs["tone"])
	}
	if defs["drive"] != 0 {
		t.Errorf("drive after Inf reject: got %v want 0 (identity)", defs["drive"])
	}
}

// TestApplyRegistersUserRecipes — a UserRecipe payload (opaque bytes)
// with a non-empty BaseRecipe field must register a new recipe id that
// resolves via NewRecipe and renders through the base recipe's renderer.
func TestApplyRegistersUserRecipes(t *testing.T) {
	const userID = "user.snare.fat.testfixture"
	t.Cleanup(func() { UnregisterRecipeForTest(userID) })

	withGate(t, true)
	doc := RecipeDoc{
		ID:          userID,
		DisplayName: "Fat Snare (Test)",
		Category:    "user",
		BaseRecipe:  "drum-snare",
		ParamDefs:   BuildBuiltinRecipeDocs()[0].ParamDefs, // any compatible param set
		ParamSeed:   RecipeParams{"decay": 2.0},
		Origin:      OriginUser,
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal doc: %v", err)
	}
	src := &stubRecipeSource{
		recipes: map[string][]byte{userID: raw},
	}
	if err := ApplyUserRecipeOverrides(src); err != nil {
		t.Fatalf("ApplyUserRecipeOverrides: %v", err)
	}
	r := NewRecipe(userID)
	if r == nil {
		t.Fatalf("NewRecipe(%q) = nil after Apply", userID)
	}
	if r.ID() != userID {
		t.Errorf("recipe ID = %q want %q", r.ID(), userID)
	}
	// Render through the user recipe; expect non-panic (under -tags test
	// the stub renderer zero-fills; under native the base C renderer
	// produces audio). The recipe_doc render tests already cover the
	// finite/byte-identity assertions — here we only assert the
	// registration path doesn't crash.
	buf := make([]float32, 1024)
	r.Render(buf, 48000, 1024, 0, RecipeDefaultParams(userID))
}

// TestApplyUnknownBaseRecipe_Skipped — a UserRecipe whose BaseRecipe
// doesn't resolve must be silently skipped (no panic, no error
// propagated), so a forward-compat doc referencing a future renderer
// can't break startup.
func TestApplyUnknownBaseRecipe_Skipped(t *testing.T) {
	const userID = "user.broken.testfixture"
	t.Cleanup(func() { UnregisterRecipeForTest(userID) })
	withGate(t, true)
	doc := RecipeDoc{
		ID:         userID,
		BaseRecipe: "does-not-exist-anywhere",
		Origin:     OriginUser,
	}
	raw, _ := json.Marshal(doc)
	src := &stubRecipeSource{recipes: map[string][]byte{userID: raw}}
	if err := ApplyUserRecipeOverrides(src); err != nil {
		t.Errorf("Apply errored on unknown base: %v", err)
	}
	if NewRecipe(userID) != nil {
		t.Errorf("user recipe with unknown base should not have been registered")
	}
}

// TestApplyNilSource_NoOp — a nil source must be a clean no-op even
// with the gate on (defensive: bootstrap paths that fail to acquire a
// store mustn't crash).
func TestApplyNilSource_NoOp(t *testing.T) {
	withGate(t, true)
	if err := ApplyUserRecipeOverrides(nil); err != nil {
		t.Errorf("Apply(nil) errored: %v", err)
	}
}

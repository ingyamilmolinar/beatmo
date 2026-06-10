//go:build !test && !js

package audio

import (
	"math"
	"testing"
)

// l1 returns the sum of absolute sample differences between two buffers.
// 0 means bit-identical content over the shared prefix.
func l1(a, b []float32) float64 {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	var d float64
	for i := 0; i < n; i++ {
		d += math.Abs(float64(a[i]) - float64(b[i]))
	}
	return d
}

// TestSynthSavePersists_RenderUnchangedAfterSave is the faithful root-cause
// test for the reported bug: turning a knob produces a liked sound, but after
// Save the sound reverts. It drives the NATIVE dispatcher (newRecipeAwareVoice
// — the -tags test stub Play is a no-op and cannot exercise this) and proves
// that the post-save / reload state (recipe defaults customized, per-instrument
// overlay empty) still renders the liked tone rather than the bare baseline.
//
// Uses drum-kick: a pitched-sine renderer with no noise, so renders are
// deterministic and byte-comparable across cache invalidations.
func TestSynthSavePersists_RenderUnchangedAfterSave(t *testing.T) {
	const id = "kick"
	recipeID := RecipeForInstrument(id)
	if recipeID == "" {
		t.Fatalf("no recipe bound to %q (fixture broken)", id)
	}

	// Snapshot original defaults and restore them on cleanup — the registry
	// is process-global, so a leaked customization would flip the gate for
	// every other test in this package.
	orig := RecipeDefaultParams(recipeID)
	t.Cleanup(func() {
		UpdateRecipeDefaultsAndInvalidate(recipeID, map[string]float64(orig))
		ResetInstrumentParams(id)
	})

	// Baseline: bare instrument, no overrides.
	ResetInstrumentParams(id)
	base := drainVoiceToBuffer(newRecipeAwareVoice(id, 120, 44100), 44100)

	// User turns the pitch knob up an octave — the "liked" sound.
	SetInstrumentParam(id, "pitch", 12)
	edited := drainVoiceToBuffer(newRecipeAwareVoice(id, 120, 44100), 44100)

	if len(base) == 0 || len(edited) == 0 {
		t.Fatalf("empty buffers: base=%d edited=%d", len(base), len(edited))
	}
	if l1(base, edited) == 0 {
		t.Fatal("sanity: pitch=+12 produced an identical buffer to baseline; knob has no effect")
	}

	// Simulate Save's audio-layer effect AND the post-restart reload state:
	// the effective params are folded into the recipe defaults, and the
	// per-instrument overlay is empty (cleared by today's Save, and always
	// empty after an app restart that reloads saved defaults from disk).
	effective := MergeRecipeDefaults(recipeID, GetInstrumentParams(id))
	UpdateRecipeDefaultsAndInvalidate(recipeID, map[string]float64(effective))
	ResetInstrumentParams(id)

	saved := drainVoiceToBuffer(newRecipeAwareVoice(id, 120, 44100), 44100)

	// The whole point of Save: the sound must NOT change. The post-save render
	// must equal the liked (edited) render and must differ from the bare
	// baseline. Pre-fix, the empty overlay drops to the legacy path which
	// ignores the saved defaults, so saved == base and saved != edited.
	if d := l1(saved, edited); d != 0 {
		t.Errorf("post-save render differs from the liked sound (L1=%v); Save changed the sound", d)
	}
	if l1(saved, base) == 0 {
		t.Error("post-save render reverted to the bare baseline; the saved recipe defaults were ignored")
	}
}

// TestSynthReset_RenderReturnsToOriginal drives the native dispatcher through a
// Save (defaults customized) then a Reset, and proves the rendered voice
// returns to the bare baseline — i.e. Reset restores the original sound even
// after a Save overwrote the recipe defaults.
func TestSynthReset_RenderReturnsToOriginal(t *testing.T) {
	const id = "kick"
	recipeID := RecipeForInstrument(id)
	if recipeID == "" {
		t.Fatalf("no recipe bound to %q (fixture broken)", id)
	}
	t.Cleanup(func() {
		ResetRecipeToShipped(recipeID)
		ResetInstrumentParams(id)
	})

	// Clean baseline.
	ResetRecipeToShipped(recipeID)
	ResetInstrumentParams(id)
	base := drainVoiceToBuffer(newRecipeAwareVoice(id, 120, 44100), 44100)

	// Edit + Save (defaults customized, overlay cleared).
	SetInstrumentParam(id, "pitch", 12)
	effective := MergeRecipeDefaults(recipeID, GetInstrumentParams(id))
	UpdateRecipeDefaultsAndInvalidate(recipeID, map[string]float64(effective))
	ResetInstrumentParams(id)
	saved := drainVoiceToBuffer(newRecipeAwareVoice(id, 120, 44100), 44100)
	if l1(saved, base) == 0 {
		t.Fatal("sanity: saved render should differ from baseline before Reset")
	}

	// Reset → restore shipped defaults + clear overlay.
	ResetRecipeToShipped(recipeID)
	ResetInstrumentParams(id)
	reset := drainVoiceToBuffer(newRecipeAwareVoice(id, 120, 44100), 44100)

	if d := l1(reset, base); d != 0 {
		t.Errorf("after Reset, render differs from the original baseline (L1=%v); Reset did not restore the original sound", d)
	}
}

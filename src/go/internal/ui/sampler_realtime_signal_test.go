package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// recipeParamsSignature is the order-independent fingerprint that drives the
// Sampler's live re-capture decision.
func TestRecipeParamsSignature(t *testing.T) {
	if recipeParamsSignature(nil) != 0 {
		t.Fatalf("nil overlay must hash to 0")
	}
	if recipeParamsSignature(audio.RecipeParams{}) != 0 {
		t.Fatalf("empty overlay must hash to 0")
	}
	base := recipeParamsSignature(audio.RecipeParams{"a": 1, "b": 2})
	if base == 0 {
		t.Fatalf("non-empty overlay must not hash to 0")
	}
	if got := recipeParamsSignature(audio.RecipeParams{"b": 2, "a": 1}); got != base {
		t.Fatalf("signature must be order-independent: got %d want %d", got, base)
	}
	if got := recipeParamsSignature(audio.RecipeParams{"a": 1, "b": 3}); got == base {
		t.Fatalf("changed value must change the signature")
	}
	if got := recipeParamsSignature(audio.RecipeParams{"a": 1}); got == base {
		t.Fatalf("dropped key must change the signature")
	}
}

// The core fix: when the source synth's signal changes (the user edits a synth
// knob), the Sampler waveform must re-render to match — without a manual Preview
// or tab switch — while keeping the user's in-progress chop.
func TestSamplerRecapturesWhenSynthSignatureChanges(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	calls := 0
	restoreCap := SwapSamplerCaptureFnForTest(func(id string) ([]float32, int) {
		calls++
		buf := make([]float32, 8)
		for i := range buf {
			buf[i] = float32(calls)
		}
		return buf, 48000
	})
	t.Cleanup(func() { SwapSamplerCaptureFnForTest(restoreCap) })

	sig := uint64(1)
	restoreSig := SwapSamplerSourceSignatureFnForTest(func(string) uint64 { return sig })
	t.Cleanup(func() { SwapSamplerSourceSignatureFnForTest(restoreSig) })

	s := &g.drum.sampler

	// Initial load renders the one-shot exactly once.
	g.drum.ensureSamplerLoaded("inst.alpha")
	if calls != 1 {
		t.Fatalf("initial load: capture calls = %d, want 1", calls)
	}
	if !s.hasBuffer() || s.raw[0] != 1 {
		t.Fatalf("initial load: raw not captured (have %v)", s.raw)
	}

	// A chop the user is mid-way through shaping.
	s.startFrac, s.endFrac, s.gainDB = 0.3, 0.7, 3

	// Same signature → no re-render (would be wasteful and would thrash the UI).
	g.drum.ensureSamplerLoaded("inst.alpha")
	if calls != 1 {
		t.Fatalf("unchanged signature: capture calls = %d, want 1", calls)
	}

	// Signature changed (synth edited) → re-render, in-progress edit preserved.
	sig = 2
	g.drum.ensureSamplerLoaded("inst.alpha")
	if calls != 2 {
		t.Fatalf("changed signature: capture calls = %d, want 2 (waveform must track the synth)", calls)
	}
	if s.raw[0] != 2 {
		t.Fatalf("changed signature: waveform not refreshed (raw[0]=%v want 2)", s.raw[0])
	}
	if s.startFrac != 0.3 || s.endFrac != 0.7 || s.gainDB != 3 {
		t.Fatalf("re-capture discarded the in-progress edit: start=%v end=%v gain=%v",
			s.startFrac, s.endFrac, s.gainDB)
	}
}

// A WAV-sourced buffer is fixed PCM — there is no synth behind it, so a
// signature change must never trigger a synth re-render.
func TestSamplerDoesNotRecaptureWAVSource(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	calls := 0
	restoreCap := SwapSamplerCaptureFnForTest(func(id string) ([]float32, int) {
		calls++
		return []float32{1, 1, 1, 1}, 48000
	})
	t.Cleanup(func() { SwapSamplerCaptureFnForTest(restoreCap) })

	sig := uint64(1)
	restoreSig := SwapSamplerSourceSignatureFnForTest(func(string) uint64 { return sig })
	t.Cleanup(func() { SwapSamplerSourceSignatureFnForTest(restoreSig) })

	s := &g.drum.sampler
	s.loadFromInstrument("user.sample.x", []float32{9, 9, 9, 9}, 48000, samplerSourceWAV)

	sig = 99 // irrelevant for a WAV source
	g.drum.ensureSamplerLoaded("user.sample.x")
	if calls != 0 {
		t.Fatalf("WAV source re-rendered from synth: capture calls = %d, want 0", calls)
	}
	if s.raw[0] != 9 {
		t.Fatalf("WAV buffer was replaced (raw[0]=%v want 9)", s.raw[0])
	}
}

// Selecting a *different* instrument is a fresh chop: edits reset to the full
// buffer (distinct from a same-instrument live re-capture, which preserves them).
func TestSamplerReloadResetsEditsOnInstrumentChange(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	restoreCap := SwapSamplerCaptureFnForTest(func(id string) ([]float32, int) {
		return []float32{1, 1, 1, 1}, 48000
	})
	t.Cleanup(func() { SwapSamplerCaptureFnForTest(restoreCap) })
	restoreSig := SwapSamplerSourceSignatureFnForTest(func(string) uint64 { return 1 })
	t.Cleanup(func() { SwapSamplerSourceSignatureFnForTest(restoreSig) })

	s := &g.drum.sampler
	g.drum.ensureSamplerLoaded("inst.alpha")
	s.startFrac, s.endFrac = 0.3, 0.7

	g.drum.ensureSamplerLoaded("inst.beta")
	if s.startFrac != 0 || s.endFrac != 1 {
		t.Fatalf("instrument change did not reset trim: start=%v end=%v", s.startFrac, s.endFrac)
	}
}

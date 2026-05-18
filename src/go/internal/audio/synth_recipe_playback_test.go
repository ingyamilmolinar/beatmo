//go:build !test && !js

package audio

import (
	"math"
	"testing"
)

// Phase 4.6 follow-up: regression tests that pin "setInstrumentParam
// during playback changes the next trigger's audio" on the desktop
// (native CGo) build path. The browser equivalent lives in
// instrument_params_audible.browser.test.js — together they cover both
// runtime targets.

// TestPlayback_SetParamChangesNextTrigger asserts that after a slider
// drag on the Synth tab (which calls audio.SetInstrumentParam), the
// very next call to newRecipeAwareVoice produces a buffer that differs
// from the pre-change rendering for the same instrument. This is the
// minimum bar: "the slider had ANY effect on the next note."
func TestPlayback_SetParamChangesNextTrigger(t *testing.T) {
	t.Cleanup(func() { ResetInstrumentParams("snare") })

	ResetInstrumentParams("snare")

	// Render once with no user params (legacy path).
	v1 := newRecipeAwareVoice("snare", 120, 44100)
	if v1 == nil {
		t.Fatal("baseline voice is nil")
	}
	baseline := drainVoiceToBuffer(v1, 44100)

	// Render again with no user params — should be cache-hit, identical bytes.
	v2 := newRecipeAwareVoice("snare", 120, 44100)
	cacheHit := drainVoiceToBuffer(v2, 44100)

	// Set a non-trivial decay value and render a third time.
	SetInstrumentParam("snare", "decay", 0.3)
	v3 := newRecipeAwareVoice("snare", 120, 44100)
	modified := drainVoiceToBuffer(v3, 44100)

	if len(baseline) == 0 || len(cacheHit) == 0 || len(modified) == 0 {
		t.Fatalf("voices produced empty buffers: %d / %d / %d", len(baseline), len(cacheHit), len(modified))
	}

	// Baseline ≈ cacheHit (variation from C noise seed allowed; the
	// envelope shape, however, should be similar). Spot-check tail RMS
	// to confirm both are full-length, full-energy renders.
	baseTail := tailRMS(baseline)
	hitTail := tailRMS(cacheHit)
	if baseTail == 0 || hitTail == 0 {
		t.Errorf("baseline or cache-hit voice has zero tail energy: base=%v hit=%v", baseTail, hitTail)
	}

	// Modified voice should have substantially LESS tail energy because
	// decay=0.3 (the recipe's identity is 1.0) shortens the envelope.
	modTail := tailRMS(modified)
	if modTail >= baseTail {
		t.Errorf("decay=0.3 should shorten envelope; got modified tailRMS=%v >= baseline tailRMS=%v", modTail, baseTail)
	}
	drop := (baseTail - modTail) / baseTail
	if drop < 0.5 {
		t.Errorf("decay=0.3 should drop tail RMS by >50%%; got %.1f%% drop (base=%v mod=%v)", drop*100, baseTail, modTail)
	}
}

// TestPlayback_ResetRestoresBaseline asserts ResetInstrumentParams
// invalidates the recipe-path cache so the next trigger renders without
// user params (i.e. produces a baseline-shaped voice again).
func TestPlayback_ResetRestoresBaseline(t *testing.T) {
	t.Cleanup(func() { ResetInstrumentParams("snare") })

	ResetInstrumentParams("snare")
	v1 := newRecipeAwareVoice("snare", 120, 44100)
	baseline := drainVoiceToBuffer(v1, 44100)
	baseTail := tailRMS(baseline)

	// Apply param, render once.
	SetInstrumentParam("snare", "decay", 0.3)
	_ = drainVoiceToBuffer(newRecipeAwareVoice("snare", 120, 44100), 44100)

	// Reset and render — should look like baseline again.
	ResetInstrumentParams("snare")
	v2 := newRecipeAwareVoice("snare", 120, 44100)
	restored := drainVoiceToBuffer(v2, 44100)
	restoredTail := tailRMS(restored)

	// Round-robin noise variation makes the byte-exact comparison
	// flaky, so we use RMS. The restored tail should be within 20% of
	// the original baseline tail.
	drift := math.Abs(restoredTail-baseTail) / math.Max(baseTail, 1e-12)
	if drift > 0.20 {
		t.Errorf("Reset did not restore baseline tail energy: base=%v restored=%v (drift %.1f%%)", baseTail, restoredTail, drift*100)
	}
}

// TestPlayback_SequentialParamChangesProduceSequentialAudio asserts
// each successive setInstrumentParam → next-render cycle produces a
// distinct voice (no stale cache between mutations). Catches the
// "cache invalidation only runs once" regression class.
func TestPlayback_SequentialParamChangesProduceSequentialAudio(t *testing.T) {
	t.Cleanup(func() { ResetInstrumentParams("snare") })

	tails := []float64{}
	for _, decay := range []float64{1.0, 0.7, 0.4, 0.2} {
		ResetInstrumentParams("snare")
		SetInstrumentParam("snare", "decay", decay)
		buf := drainVoiceToBuffer(newRecipeAwareVoice("snare", 120, 44100), 44100)
		tails = append(tails, tailRMS(buf))
	}

	// As decay shrinks, tail RMS must shrink monotonically.
	for i := 1; i < len(tails); i++ {
		if tails[i] >= tails[i-1] {
			t.Errorf("decay should reduce tail energy monotonically; tails[%d]=%v >= tails[%d]=%v",
				i, tails[i], i-1, tails[i-1])
		}
	}
}

// tailRMS computes the RMS of the back half of the buffer; used by the
// Phase 4.6 audible-change regression suite as a proxy for "did the
// envelope change shape?"
func tailRMS(buf []float32) float64 {
	if len(buf) < 2 {
		return 0
	}
	start := len(buf) / 2
	var sumSq float64
	for i := start; i < len(buf); i++ {
		f := float64(buf[i])
		sumSq += f * f
	}
	return math.Sqrt(sumSq / float64(len(buf)-start))
}

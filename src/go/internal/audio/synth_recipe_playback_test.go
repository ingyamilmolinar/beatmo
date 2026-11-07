//go:build !test && !js

package audio

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// The native half; instrument_params_audible.browser.test.js covers the browser.

// The minimum bar: the slider had ANY effect on the next note.
func TestPlayback_SetParamChangesNextTrigger(t *testing.T) {
	t.Cleanup(func() { ResetInstrumentParams(roster.Snare) })

	ResetInstrumentParams(roster.Snare)

	// Render once with no user params (legacy path).
	v1 := newRecipeAwareVoice(roster.Snare, 44100)
	if v1 == nil {
		t.Fatal("baseline voice is nil")
	}
	baseline := drainVoiceToBuffer(v1, 44100)

	// Render again with no user params — should be cache-hit, identical bytes.
	v2 := newRecipeAwareVoice(roster.Snare, 44100)
	cacheHit := drainVoiceToBuffer(v2, 44100)

	// Set a non-trivial decay value and render a third time.
	SetInstrumentParam(roster.Snare, "post_decay", 0.3)
	v3 := newRecipeAwareVoice(roster.Snare, 44100)
	modified := drainVoiceToBuffer(v3, 44100)

	if len(baseline) == 0 || len(cacheHit) == 0 || len(modified) == 0 {
		t.Fatalf("voices produced empty buffers: %d / %d / %d", len(baseline), len(cacheHit), len(modified))
	}

	// The C noise seed varies, so tail RMS checks both are full-length, full-energy renders.
	baseTail := tailRMS(baseline)
	hitTail := tailRMS(cacheHit)
	if baseTail == 0 || hitTail == 0 {
		t.Errorf("baseline or cache-hit voice has zero tail energy: base=%v hit=%v", baseTail, hitTail)
	}

	// post_decay 0.3 (identity 1.0) shortens the post envelope.
	modTail := tailRMS(modified)
	if modTail >= baseTail {
		t.Errorf("post_decay=0.3 should shorten envelope; got modified tailRMS=%v >= baseline tailRMS=%v", modTail, baseTail)
	}
	drop := (baseTail - modTail) / baseTail
	if drop < 0.5 {
		t.Errorf("post_decay=0.3 should drop tail RMS by >50%%; got %.1f%% drop (base=%v mod=%v)", drop*100, baseTail, modTail)
	}
}

func TestPlayback_ResetRestoresBaseline(t *testing.T) {
	t.Cleanup(func() { ResetInstrumentParams(roster.Snare) })

	ResetInstrumentParams(roster.Snare)
	v1 := newRecipeAwareVoice(roster.Snare, 44100)
	baseline := drainVoiceToBuffer(v1, 44100)
	baseTail := tailRMS(baseline)

	// Apply param, render once.
	SetInstrumentParam(roster.Snare, "post_decay", 0.3)
	_ = drainVoiceToBuffer(newRecipeAwareVoice(roster.Snare, 44100), 44100)

	// Reset and render — should look like baseline again.
	ResetInstrumentParams(roster.Snare)
	v2 := newRecipeAwareVoice(roster.Snare, 44100)
	restored := drainVoiceToBuffer(v2, 44100)
	restoredTail := tailRMS(restored)

	// Round-robin noise makes bytes flaky, so RMS within 20 %.
	drift := math.Abs(restoredTail-baseTail) / math.Max(baseTail, 1e-12)
	if drift > 0.20 {
		t.Errorf("Reset did not restore baseline tail energy: base=%v restored=%v (drift %.1f%%)", baseTail, restoredTail, drift*100)
	}
}

// No stale cache between mutations.
func TestPlayback_SequentialParamChangesProduceSequentialAudio(t *testing.T) {
	t.Cleanup(func() { ResetInstrumentParams(roster.Snare) })

	tails := []float64{}
	for _, decay := range []float64{1.0, 0.7, 0.4, 0.2} {
		ResetInstrumentParams(roster.Snare)
		SetInstrumentParam(roster.Snare, "post_decay", decay)
		buf := drainVoiceToBuffer(newRecipeAwareVoice(roster.Snare, 44100), 44100)
		tails = append(tails, tailRMS(buf))
	}

	// As decay shrinks, tail RMS must shrink monotonically.
	for i := 1; i < len(tails); i++ {
		if tails[i] >= tails[i-1] {
			t.Errorf("post_decay should reduce tail energy monotonically; tails[%d]=%v >= tails[%d]=%v",
				i, tails[i], i-1, tails[i-1])
		}
	}
}

// A proxy for "did the envelope change shape?".
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

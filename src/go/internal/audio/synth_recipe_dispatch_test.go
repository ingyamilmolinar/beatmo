//go:build !test && !js

package audio

import (
	"math"
	"testing"
)

// TestRecipeDispatch_NoUserParamsFallsBackToLegacy asserts that when no
// per-instrument params are set, the dispatcher returns the same kind of
// voice as the legacy Instrument.NewVoice path.
func TestRecipeDispatch_NoUserParamsFallsBackToLegacy(t *testing.T) {
	t.Cleanup(func() { ResetInstrumentParams("snare") })

	ResetInstrumentParams("snare") // belt-and-suspenders: ensure clean state
	v := newRecipeAwareVoice("snare", 120, 44100)
	if v == nil {
		t.Fatal("dispatcher returned nil for default-param snare")
	}
	if _, ok := v.(*cVoice); !ok {
		t.Errorf("legacy fallback should produce *cVoice; got %T", v)
	}
}

// TestRecipeDispatch_UserParamsRouteThroughRecipe asserts that the recipe
// path produces a different buffer than the legacy path for the same
// instrument when user params are set. This is the v1 freezing-surface
// for "SetInstrumentParam changes audio output."
func TestRecipeDispatch_UserParamsRouteThroughRecipe(t *testing.T) {
	t.Cleanup(func() { ResetInstrumentParams("snare") })

	// Render via the legacy path (no params).
	ResetInstrumentParams("snare")
	legacy := newRecipeAwareVoice("snare", 120, 44100)
	legacyBuf := drainVoiceToBuffer(legacy, 44100)

	// Set non-default params and render via the recipe path.
	SetInstrumentParam("snare", "decay", 0.4)
	recipe := newRecipeAwareVoice("snare", 120, 44100)
	recipeBuf := drainVoiceToBuffer(recipe, 44100)

	if len(legacyBuf) == 0 || len(recipeBuf) == 0 {
		t.Fatalf("empty buffers: legacy=%d recipe=%d", len(legacyBuf), len(recipeBuf))
	}
	// They should diverge somewhere in the buffer; a non-zero L1 distance
	// is enough to confirm the recipe ran with the new decay value.
	var dist float64
	n := len(legacyBuf)
	if len(recipeBuf) < n {
		n = len(recipeBuf)
	}
	for i := 0; i < n; i++ {
		dist += math.Abs(float64(legacyBuf[i]) - float64(recipeBuf[i]))
	}
	if dist == 0 {
		t.Error("recipe path produced identical buffer to legacy path despite non-default decay")
	}
}

// TestRecipeDispatch_UnknownInstrumentReturnsNil mirrors the silent-drop
// behavior of legacyNewVoice for unknown ids; the dispatcher must not
// panic or invent a buffer.
func TestRecipeDispatch_UnknownInstrumentReturnsNil(t *testing.T) {
	if v := newRecipeAwareVoice("not-a-real-instrument", 120, 44100); v != nil {
		t.Errorf("expected nil for unknown instrument; got %#v", v)
	}
}

// TestRecipeDispatch_CacheHitOnRepeatedTrigger asserts that consecutive
// triggers with the same params return cached buffers (one render call,
// then cache hits).
func TestRecipeDispatch_CacheHitOnRepeatedTrigger(t *testing.T) {
	t.Cleanup(func() { ResetInstrumentParams("snare") })

	ResetInstrumentParams("snare")
	SetInstrumentParam("snare", "decay", 0.7)

	// First call renders + caches.
	v1 := newRecipeAwareVoice("snare", 120, 44100)
	buf1 := drainVoiceToBuffer(v1, 1024)

	// Second call should hit the cache → identical buffer.
	v2 := newRecipeAwareVoice("snare", 120, 44100)
	buf2 := drainVoiceToBuffer(v2, 1024)

	if len(buf1) != len(buf2) {
		t.Fatalf("buffer lengths differ: %d vs %d", len(buf1), len(buf2))
	}
	for i := range buf1 {
		if buf1[i] != buf2[i] {
			t.Errorf("cache miss on second trigger at sample %d: %v vs %v", i, buf1[i], buf2[i])
			break
		}
	}
}

// TestRecipeDispatch_InstrumentDurationSamples covers the dual-mode
// duration calc. CVariantInstrument variants (e.g. snare-1 with Beats=0.5)
// derive duration from BPM; legacy types (e.g. snare with no Beats) use
// ConfigForInstrument's fixed DurationSec.
func TestRecipeDispatch_InstrumentDurationSamples(t *testing.T) {
	// snare is a legacy type → fixed duration, bpm=0 in key
	got, bpmKey := instrumentDurationSamples("snare", 120, 44100)
	if got <= 0 {
		t.Errorf("snare samples=%d; expected positive", got)
	}
	if bpmKey != 0 {
		t.Errorf("snare bpmKey=%d; expected 0 (fixed duration)", bpmKey)
	}

	// snare-1 is a CVariantInstrument with Beats=0.5 → BPM-derived
	got1, bpmKey1 := instrumentDurationSamples("snare-1", 120, 44100)
	if got1 <= 0 {
		t.Errorf("snare-1 samples=%d; expected positive", got1)
	}
	if bpmKey1 != 120 {
		t.Errorf("snare-1 bpmKey=%d; expected 120 (BPM-derived)", bpmKey1)
	}

	// Doubling BPM should halve sample count for a CVariantInstrument.
	got2, _ := instrumentDurationSamples("snare-1", 240, 44100)
	if got2 >= got1 {
		t.Errorf("snare-1 @240bpm samples=%d >= @120bpm %d; expected shorter at faster tempo", got2, got1)
	}
}

// drainVoiceToBuffer pulls up to N samples from v and returns them as
// float32. Used by recipe-dispatch tests to compare voice outputs.
func drainVoiceToBuffer(v Voice, n int) []float32 {
	if v == nil {
		return nil
	}
	out := make([]float32, 0, n)
	for i := 0; i < n; i++ {
		s, done := v.Sample()
		out = append(out, float32(s))
		if done {
			break
		}
	}
	return out
}

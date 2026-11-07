//go:build !test && !js

package audio

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

func TestRecipeDispatch_NoUserParamsFallsBackToLegacy(t *testing.T) {
	t.Cleanup(func() { ResetInstrumentParams(roster.Snare) })

	ResetInstrumentParams(roster.Snare) // belt-and-suspenders: ensure clean state
	v := newRecipeAwareVoice(roster.Snare, 44100)
	if v == nil {
		t.Fatal("dispatcher returned nil for default-param snare")
	}
	if _, ok := v.(*cVoice); !ok {
		t.Errorf("legacy fallback should produce *cVoice; got %T", v)
	}
}

// The minimum contract: SetInstrumentParam changes audio output.
func TestRecipeDispatch_UserParamsRouteThroughRecipe(t *testing.T) {
	t.Cleanup(func() { ResetInstrumentParams(roster.Snare) })

	// Render via the legacy path (no params).
	ResetInstrumentParams(roster.Snare)
	legacy := newRecipeAwareVoice(roster.Snare, 44100)
	legacyBuf := drainVoiceToBuffer(legacy, 44100)

	// Set non-default params and render via the recipe path.
	SetInstrumentParam(roster.Snare, "post_decay", 0.4)
	recipe := newRecipeAwareVoice(roster.Snare, 44100)
	recipeBuf := drainVoiceToBuffer(recipe, 44100)

	if len(legacyBuf) == 0 || len(recipeBuf) == 0 {
		t.Fatalf("empty buffers: legacy=%d recipe=%d", len(legacyBuf), len(recipeBuf))
	}
	var dist float64
	n := len(legacyBuf)
	if len(recipeBuf) < n {
		n = len(recipeBuf)
	}
	for i := 0; i < n; i++ {
		dist += math.Abs(float64(legacyBuf[i]) - float64(recipeBuf[i]))
	}
	if dist == 0 {
		t.Error("recipe path produced identical buffer to legacy path despite non-default post_decay")
	}
}

// Must not panic or invent a buffer.
func TestRecipeDispatch_UnknownInstrumentReturnsNil(t *testing.T) {
	if v := newRecipeAwareVoice("not-a-real-instrument", 44100); v != nil {
		t.Errorf("expected nil for unknown instrument; got %#v", v)
	}
}

// One render, then hits.
func TestRecipeDispatch_CacheHitOnRepeatedTrigger(t *testing.T) {
	t.Cleanup(func() { ResetInstrumentParams(roster.Snare) })

	ResetInstrumentParams(roster.Snare)
	SetInstrumentParam(roster.Snare, "post_decay", 0.7)

	// First call renders + caches.
	v1 := newRecipeAwareVoice(roster.Snare, 44100)
	buf1 := drainVoiceToBuffer(v1, 1024)

	// Second call should hit the cache → identical buffer.
	v2 := newRecipeAwareVoice(roster.Snare, 44100)
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

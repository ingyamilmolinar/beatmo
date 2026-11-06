//go:build !test && !js

package audio

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// TestSecondWarmOfIdenticalSetRendersNothing pins the headline cache-hit
// guarantee: re-loading a document whose instruments, params and pitches are
// unchanged must render ZERO voices. This is the promise the import-warmup
// playback gate is built on — without it, every re-import and every template
// reload pays full render cost again.
func TestSecondWarmOfIdenticalSetRendersNothing(t *testing.T) {
	pitches := []float64{-12, -5, 0, 4, 7, 12}
	for _, p := range pitches {
		PrewarmVoice(roster.Cello, p)
	}
	ResetRenderLatencyMetrics()
	for _, p := range pitches {
		if !VoiceCached(roster.Cello, p) {
			t.Fatalf("cello@%v not cached after the first warm", p)
		}
		PrewarmVoice(roster.Cello, p)
	}
	if m := GetRenderLatencyMetrics(); m.CacheMisses != 0 {
		t.Errorf("second warm of an identical set rendered %d voices, want 0", m.CacheMisses)
	}
}

// TestEffectsChangeDoesNotInvalidateVoices pins that insert effects are
// post-voice processing and are correctly ABSENT from voiceCacheKey. Changing
// only the effect chain must not re-render a single voice — if this
// ever fails, someone "fixed" the key by adding effects to it and made every
// FX tweak cost a full circuit re-render.
func TestEffectsChangeDoesNotInvalidateVoices(t *testing.T) {
	const id = roster.Violin
	PrewarmVoice(id, 0)
	PrewarmVoice(id, 7)
	ResetRenderLatencyMetrics()

	SetInsertEffects(id, []EffectSlot{{Type: EffectDelay, Enabled: true,
		Params: map[string]float64{"mix": 0.4}}})

	if !VoiceCached(id, 0) || !VoiceCached(id, 7) {
		t.Fatal("an effects change invalidated the voice cache")
	}
	PrewarmVoice(id, 0)
	PrewarmVoice(id, 7)
	if m := GetRenderLatencyMetrics(); m.CacheMisses != 0 {
		t.Errorf("effects change caused %d voice re-renders, want 0", m.CacheMisses)
	}
}

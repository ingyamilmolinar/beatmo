//go:build !test && !js

package audio

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// The promise the import warm-up gate is built on: an unchanged document re-renders ZERO voices.
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

// Insert effects are post-voice and ABSENT from the key: adding them would make every FX tweak re-render the circuit.
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

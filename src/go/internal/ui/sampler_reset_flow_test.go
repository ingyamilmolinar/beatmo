//go:build test

package ui

import (
	"sync"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

// captureSampleResetHooks subscribes to EventSampleReset and returns an
// accessor; hooks delivery is async so callers pair it with waitForKind.
func captureSampleResetHooks(t *testing.T) func() []hooks.Event {
	t.Helper()
	var mu sync.Mutex
	var seen []hooks.Event
	unsub := hooks.Subscribe(hooks.EventSampleReset, func(e hooks.Event) {
		mu.Lock()
		seen = append(seen, e)
		mu.Unlock()
	})
	t.Cleanup(unsub)
	return func() []hooks.Event {
		mu.Lock()
		defer mu.Unlock()
		out := make([]hooks.Event, len(seen))
		copy(out, seen)
		return out
	}
}

// TestSamplerReset_BuiltinRevertsToSynth — Reset on a built-in instrument that
// was converted to a sample by the LEGACY destructive Save (pre-descriptor:
// origin recorded, binding cleared, PCM baked — state that can still arrive
// from old persisted sessions) re-binds the original synth recipe and discards
// the user sample (full factory revert). The new Save is non-destructive (see
// sampler_edit_descriptor_flow_test.go), so the converted state is staged
// directly through the audio APIs the old flow used.
func TestSamplerReset_BuiltinRevertsToSynth(t *testing.T) {
	g := newSamplerTabGame(t)
	orig := audio.RecipeForInstrument("kick")
	t.Cleanup(func() { audio.BindInstrumentToRecipe("kick", orig) })
	audio.BindInstrumentToRecipe("kick", "drum-kick")

	g.drum.ensureSamplerLoaded("kick")
	// Stage the legacy-converted state (what the old destructive Save wrote).
	audio.RecordSampleOriginRecipe("kick", "drum-kick")
	audio.BindInstrumentToRecipe("kick", "")
	baked, sr := g.drum.sampler.bake()
	audio.SaveUserSample("kick", baked, sr)
	if audio.RecipeForInstrument("kick") != "" || !audio.IsUserSample("kick") {
		t.Fatalf("sanity: legacy-converted state not staged")
	}
	g.drum.sampler.captureID = "kick"

	g.drum.samplerReset()

	if got := audio.RecipeForInstrument("kick"); got != "drum-kick" {
		t.Errorf("after Reset RecipeForInstrument(kick)=%q, want drum-kick", got)
	}
	if audio.IsUserSample("kick") {
		t.Errorf("after Reset kick is still a user sample, want discarded")
	}
}

// TestSamplerReset_UserSampleRestoresOriginal — Reset on a user WAV/Save-As
// sample restores its pristine first-loaded buffer.
func TestSamplerReset_UserSampleRestoresOriginal(t *testing.T) {
	g := newSamplerTabGame(t)
	const id = "user.sample.resettest"
	pcmA := make([]float32, 800)
	for i := range pcmA {
		pcmA[i] = 0.2
	}
	audio.SaveUserSample(id, pcmA, 48000) // original
	g.drum.ensureSamplerLoaded(id)

	pcmB := make([]float32, 400)
	for i := range pcmB {
		pcmB[i] = 0.9
	}
	audio.SaveUserSample(id, pcmB, 48000) // edited re-save

	g.drum.samplerReset()

	cur, ok := audio.UserSamplePCM(id)
	if !ok || len(cur.PCM) != len(pcmA) || cur.PCM[0] != 0.2 {
		t.Errorf("after Reset buffer len=%d head=%v, want pristine original (len %d, 0.2)", len(cur.PCM), cur.PCM[:1], len(pcmA))
	}
}

// TestSamplerReset_ReloadsBufferAfterReset — the handler clears captureID so the
// next Layout re-pulls the reverted buffer / synth one-shot (ensureSamplerLoaded
// early-returns while captureID matches a loaded buffer).
func TestSamplerReset_ReloadsBufferAfterReset(t *testing.T) {
	g := newSamplerTabGame(t)
	const id = "user.sample.reloadtest"
	pcm := make([]float32, 600)
	audio.SaveUserSample(id, pcm, 48000)
	g.drum.ensureSamplerLoaded(id)
	if g.drum.sampler.captureID != id {
		t.Fatalf("sanity: captureID=%q, want %q", g.drum.sampler.captureID, id)
	}

	g.drum.samplerReset()

	if g.drum.sampler.captureID != "" {
		t.Errorf("after Reset captureID=%q, want cleared so buffer reloads", g.drum.sampler.captureID)
	}
}

// TestSamplerReset_PublishesEvent — Reset publishes EventSampleReset carrying
// the instrument id and the recipe it reverted to.
func TestSamplerReset_PublishesEvent(t *testing.T) {
	events := captureSampleResetHooks(t)
	g := newSamplerTabGame(t)
	orig := audio.RecipeForInstrument("kick")
	t.Cleanup(func() { audio.BindInstrumentToRecipe("kick", orig) })
	audio.BindInstrumentToRecipe("kick", "drum-kick")

	g.drum.ensureSamplerLoaded("kick")
	g.drum.sampler.save()
	g.drum.samplerReset()

	e := waitForKind(t, events, hooks.EventSampleReset)
	p, ok := e.Payload.(hooks.SamplePayload)
	if !ok {
		t.Fatalf("payload type = %T, want hooks.SamplePayload", e.Payload)
	}
	if p.SampleID != "kick" || p.SourceID != "drum-kick" {
		t.Errorf("payload = {SampleID:%q SourceID:%q}, want {kick drum-kick}", p.SampleID, p.SourceID)
	}
}

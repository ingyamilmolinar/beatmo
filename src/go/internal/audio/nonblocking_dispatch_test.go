//go:build !test && !js

package audio

import "testing"

// INVARIANT I3: with non-blocking dispatch armed, a cache miss must NEVER
// render on the caller's goroutine. It must serve the best available buffer and
// hand the exact render off-thread.
func TestNonBlockingDispatchNeverRendersInline(t *testing.T) {
	const id = "organ-church"
	globalVoiceCache.Clear()
	ResetInstrumentParams(id)
	t.Cleanup(func() {
		ResetInstrumentParams(id)
		dispatchMustNotRender.Store(false)
		SetAsyncRenderSubmit(nil)
	})

	// Warm ONE binding at pitch 0 so a best-available candidate exists.
	PrewarmVoice(id, 0)

	var submitted int
	SetAsyncRenderSubmit(func(id string, pitch float64, params RecipeParams) { submitted++ })
	dispatchMustNotRender.Store(true)

	// Rotate the binding so the next trigger is a guaranteed miss.
	SetInstrumentParam(id, churnableParam(t, id), 0.61)

	ResetRenderLatencyMetrics()
	v := newRecipeAwareVoicePitched(id, bpm, SampleRate(), 0)
	m := GetRenderLatencyMetrics()

	if v == nil {
		t.Fatal("dispatch returned no voice — a miss must still produce audio, on time")
	}
	if m.DispatchRenders != 0 {
		t.Errorf("dispatch rendered inline despite the non-blocking gate: DispatchRenders=%d want 0", m.DispatchRenders)
	}
	if m.StalePlays != 1 {
		t.Errorf("StalePlays=%d want 1 — a best-available fallback must be counted, "+
			"otherwise a permanently-stale instrument looks healthy", m.StalePlays)
	}
	if submitted != 1 {
		t.Errorf("asyncRenderSubmit called %d times, want 1 — without the hand-off "+
			"the exact binding never renders and the instrument stays stale forever", submitted)
	}
}

// With the gate DISARMED (e.g. prewarm pool calls), rendering inline is correct.
func TestNonBlockingGateDoesNotAffectPrewarm(t *testing.T) {
	const id = "organ-church"
	globalVoiceCache.Clear()
	dispatchMustNotRender.Store(false)
	t.Cleanup(func() { dispatchMustNotRender.Store(false) })

	ResetRenderLatencyMetrics()
	PrewarmVoice(id, 4)
	if m := GetRenderLatencyMetrics(); m.PrewarmRenders != 1 {
		t.Fatalf("prewarm must still render: PrewarmRenders=%d want 1", m.PrewarmRenders)
	}
}

// The nothing-cached fall-through (user-mandated): with the non-blocking gate
// ARMED but NOTHING cached for the instrument, serveBestAvailable returns false
// and dispatch must FALL THROUGH to a real render — a LATE first note, never
// silence and never nil. Regression net for the Tier-3→fall-through revert.
func TestNonBlockingDispatchFallsThroughWhenNothingCached(t *testing.T) {
	const id = "organ-church"
	globalVoiceCache.Clear()
	ResetInstrumentParams(id)
	t.Cleanup(func() {
		ResetInstrumentParams(id)
		dispatchMustNotRender.Store(false)
		SetAsyncRenderSubmit(nil)
	})

	SetAsyncRenderSubmit(func(string, float64, RecipeParams) {})
	dispatchMustNotRender.Store(true)

	ResetRenderLatencyMetrics()
	v := newRecipeAwareVoicePitched(id, bpm, SampleRate(), 0)
	m := GetRenderLatencyMetrics()

	if v == nil {
		t.Fatal("nothing-cached fall-through returned nil — a first note must still sound")
	}
	if m.DispatchRenders != 1 {
		t.Errorf("expected fall-through to RENDER on dispatch (DispatchRenders=1), got %d — "+
			"if 0, it silently served empty/stale instead of the mandated late render", m.DispatchRenders)
	}
	if m.StalePlays != 0 {
		t.Errorf("StalePlays=%d — nothing was cached, so best-available must NOT have fired", m.StalePlays)
	}
	cv, ok := v.(*cVoice)
	if !ok || len(cv.buf) == 0 {
		t.Fatalf("expected a non-empty *cVoice, got %T", v)
	}
	nonzero := false
	for _, s := range cv.buf {
		if s != 0 {
			nonzero = true
			break
		}
	}
	if !nonzero {
		t.Error("fall-through voice is ALL SILENCE — the mandated late-render produced no audio " +
			"(this is exactly the Tier-3 silence behavior the user rejected)")
	}
}

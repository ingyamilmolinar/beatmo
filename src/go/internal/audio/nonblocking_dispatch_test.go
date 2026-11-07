//go:build !test && !js

package audio

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// With non-blocking dispatch armed, a miss serves the best available buffer and renders off-thread.
func TestNonBlockingDispatchNeverRendersInline(t *testing.T) {
	const id = roster.OrganChurch
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
	SetAsyncRenderSubmit(func(id instrument.ID, pitch float64, d instrument.Doc) { submitted++ })
	dispatchMustNotRender.Store(true)

	// Rotate the binding so the next trigger is a guaranteed miss.
	SetInstrumentParam(id, churnableParam(t, string(id)), 0.61)

	ResetRenderLatencyMetrics()
	v := newRecipeAwareVoicePitched(id, SampleRate(), 0)
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
	const id = roster.OrganChurch
	globalVoiceCache.Clear()
	dispatchMustNotRender.Store(false)
	t.Cleanup(func() { dispatchMustNotRender.Store(false) })

	ResetRenderLatencyMetrics()
	PrewarmVoice(id, 4)
	if m := GetRenderLatencyMetrics(); m.PrewarmRenders != 1 {
		t.Fatalf("prewarm must still render: PrewarmRenders=%d want 1", m.PrewarmRenders)
	}
}

// With nothing cached the dispatch falls through to a real render: a late first note, never silence.
func TestNonBlockingDispatchFallsThroughWhenNothingCached(t *testing.T) {
	const id = roster.OrganChurch
	globalVoiceCache.Clear()
	ResetInstrumentParams(id)
	t.Cleanup(func() {
		ResetInstrumentParams(id)
		dispatchMustNotRender.Store(false)
		SetAsyncRenderSubmit(nil)
	})

	SetAsyncRenderSubmit(func(instrument.ID, float64, instrument.Doc) {})
	dispatchMustNotRender.Store(true)

	ResetRenderLatencyMetrics()
	v := newRecipeAwareVoicePitched(id, SampleRate(), 0)
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

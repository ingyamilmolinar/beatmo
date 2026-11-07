package ui

import (
	"sync"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Exactly one audition per call through the production trigger plumbing, never for an empty id; only Preview calls
// it. Through the synthAuditionFn hook, with no real engine.

func TestAuditionInstrumentAfterDrag_FiresOncePerRelease(t *testing.T) {
	dv := &DrumView{}
	var mu sync.Mutex
	var calls []string
	prev := SwapSynthAuditionFnForTest(func(id instrument.ID) {
		mu.Lock()
		calls = append(calls, string(id))
		mu.Unlock()
	})
	t.Cleanup(func() { SwapSynthAuditionFnForTest(prev) })

	// Three releases on different instruments.
	dv.auditionInstrumentAfterDrag(roster.Snare)
	dv.auditionInstrumentAfterDrag(roster.Kick)
	dv.auditionInstrumentAfterDrag(roster.Hihat)

	mu.Lock()
	defer mu.Unlock()
	if got, want := len(calls), 3; got != want {
		t.Fatalf("audition call count = %d, want %d (calls=%v)", got, want, calls)
	}
	wantSeq := []instrument.ID{roster.Snare, roster.Kick, roster.Hihat}
	for i, want := range wantSeq {
		if instrument.ID(calls[i]) != want {
			t.Errorf("call %d: got %q, want %q", i, calls[i], want)
		}
	}
}

func TestAuditionInstrumentAfterDrag_NoOpOnEmptyID(t *testing.T) {
	dv := &DrumView{}
	var calls int
	prev := SwapSynthAuditionFnForTest(func(instrument.ID) { calls++ })
	t.Cleanup(func() { SwapSynthAuditionFnForTest(prev) })

	// Play("") might one day mean "play default".
	dv.auditionInstrumentAfterDrag("")

	if calls != 0 {
		t.Errorf("audition fired for empty id (calls=%d, want 0)", calls)
	}
}

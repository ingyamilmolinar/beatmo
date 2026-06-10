package ui

import (
	"sync"
	"testing"
)

// Helper contract: auditionInstrumentAfterDrag fires exactly one audition
// trigger (audio.Play) per call for the resolved instrument. Its only
// production caller is the explicit Preview button (previewActiveSynth) —
// knob releases and stage toggles are config-only (next-trigger model; see
// synth_no_auto_audition_test.go). The audition reuses the production
// trigger plumbing (effect chain, voice cache invalidation, send-FX) — no
// parallel preview voice.
//
// We test through the swappable synthAuditionFn hook so the test never
// has to spin up the real engine. Each scenario:
//
//   - drives auditionInstrumentAfterDrag directly with the expected
//     instrument id; asserts the hook fires exactly once with that id.
//   - asserts the hook is NOT called when instID is empty (resolveSynth
//     can return "" if the active row has no instrument).

func TestAuditionInstrumentAfterDrag_FiresOncePerRelease(t *testing.T) {
	dv := &DrumView{}
	var mu sync.Mutex
	var calls []string
	prev := SwapSynthAuditionFnForTest(func(id string) {
		mu.Lock()
		calls = append(calls, id)
		mu.Unlock()
	})
	t.Cleanup(func() { SwapSynthAuditionFnForTest(prev) })

	// Each release fires exactly once. Simulate three independent drag
	// releases on different instruments.
	dv.auditionInstrumentAfterDrag("snare")
	dv.auditionInstrumentAfterDrag("kick")
	dv.auditionInstrumentAfterDrag("hihat")

	mu.Lock()
	defer mu.Unlock()
	if got, want := len(calls), 3; got != want {
		t.Fatalf("audition call count = %d, want %d (calls=%v)", got, want, calls)
	}
	wantSeq := []string{"snare", "kick", "hihat"}
	for i, want := range wantSeq {
		if calls[i] != want {
			t.Errorf("call %d: got %q, want %q", i, calls[i], want)
		}
	}
}

func TestAuditionInstrumentAfterDrag_NoOpOnEmptyID(t *testing.T) {
	dv := &DrumView{}
	var calls int
	prev := SwapSynthAuditionFnForTest(func(string) { calls++ })
	t.Cleanup(func() { SwapSynthAuditionFnForTest(prev) })

	// Empty instrument id (e.g. an unbound row) must not trigger audition —
	// otherwise the audio engine receives a Play("") which legacy code may
	// silently drop, but worse, future code may interpret as "play default".
	dv.auditionInstrumentAfterDrag("")

	if calls != 0 {
		t.Errorf("audition fired for empty id (calls=%d, want 0)", calls)
	}
}

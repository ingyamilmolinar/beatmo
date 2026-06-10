package scope

import (
	"testing"
)

// TestTickNoTapsZeroDrainAllocs pins the per-tick allocation count for
// the scope service when no taps are set — the dominant configuration
// during the synth-tab playback session that triggered the OOM
// investigation. Before the fix every tick called ringBuf.drain on all
// six stages, each call allocating a ~172 KB []float64 only to discard
// it. The synth-tab profile attributed ~508 MB of cumulative allocations
// over 60 s to (*Service).tick.
//
// After the fix unused stages (any stage that is not the current tap A
// or tap B) are cleared in-place via ringBuf.clear(), which is
// allocation-free. With no taps set, ALL six stages take the clear path
// and tick allocates zero bytes per call.
//
// Audio data is pre-populated in every ring so the test exercises the
// "stage has data" path (the empty-ring path returns early and would
// trivially pass).
func TestTickNoTapsZeroDrainAllocs(t *testing.T) {
	svc := NewService(Config{
		MaxWindowMs: 500,
		SampleRate:  44100,
	})
	// Default: taps unset (atomic.Int32 default = 0, but the constructor
	// stores -1 — verify here so the test fails loudly if that contract
	// changes).
	if svc.TapA() != -1 || svc.TapB() != -1 {
		t.Fatalf("expected no taps set; got TapA=%d TapB=%d", svc.TapA(), svc.TapB())
	}

	// Push to every stage so each ring has buffered samples for tick to
	// process. Push a slice the audio thread would realistically deliver
	// (one 512-sample block × ~22 blocks/s would accumulate ~11 K samples
	// in 33 ms; we use 4096 to keep the test fast).
	samples := make([]float64, 4096)
	for i := range samples {
		samples[i] = float64(i)
	}
	pushAll := func() {
		for stage := Stage(0); stage < stageCount; stage++ {
			svc.PushSamples(stage, "kick", samples)
		}
	}
	pushAll()

	// Warm up — the first tick may pay one-time costs (atomic.Pointer
	// type assertion, prev=nil branch, etc.).
	for i := 0; i < 5; i++ {
		svc.tick()
		pushAll()
	}

	const budget = 0.0
	allocs := testing.AllocsPerRun(50, func() {
		// Refill rings before each measured tick so the "ring has data"
		// branch is exercised. PushSamples may legitimately reallocate
		// the ring's backing buffer the first time but reaches steady
		// state after warmup; AllocsPerRun's outer loop runs the closure
		// many times, so any post-steady-state alloc trips the budget.
		pushAll()
		svc.tick()
	})
	if allocs > budget {
		t.Fatalf("tick allocs/call = %.1f exceeds budget %.0f — "+
			"a code path is allocating per-tick when no taps are set. "+
			"Unused stages must clear() not drain(); drain allocates a "+
			"maxSamples slice (~172 KB) that is then discarded. See "+
			"internal/scope/service.go tick().",
			allocs, budget)
	}
	t.Logf("tick allocs/call = %.1f (budget %.0f, %d stages, no taps set) — "+
		"pre-fix this was ~6 allocs/tick (one per stage drain).",
		allocs, budget, int(stageCount))
}

// TestTickWithOneTapBoundedAllocs verifies that even when a tap is set,
// the per-tick allocation count stays small — at most two allocs (the
// drain output slice for the tapped stage + the published State struct).
// Five unused stages still take the zero-alloc clear path.
func TestTickWithOneTapBoundedAllocs(t *testing.T) {
	svc := NewService(Config{
		MaxWindowMs: 500,
		SampleRate:  44100,
	})
	svc.SetTapA(StageSynth)

	samples := make([]float64, 4096)
	for i := range samples {
		samples[i] = float64(i)
	}
	pushAll := func() {
		for stage := Stage(0); stage < stageCount; stage++ {
			svc.PushSamples(stage, "kick", samples)
		}
	}
	pushAll()
	for i := 0; i < 5; i++ {
		svc.tick()
		pushAll()
	}

	// Drain alloc + State struct alloc = 2. (buildTapData returns a
	// value-type TapData so no heap alloc; the State is heap because
	// it's stored via atomic.Pointer.) Allow a slack of 1 for the
	// returned drain slice — depending on Go version the slice header
	// for the variadic samples... arg in push counts may shift by one.
	const budget = 3.0
	allocs := testing.AllocsPerRun(50, func() {
		pushAll()
		svc.tick()
	})
	if allocs > budget {
		t.Fatalf("tick (one tap) allocs/call = %.1f exceeds budget %.0f", allocs, budget)
	}
	t.Logf("tick (one tap) allocs/call = %.1f (budget %.0f)", allocs, budget)
}

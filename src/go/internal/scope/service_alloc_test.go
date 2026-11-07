package scope

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// With no taps (the common case) every stage takes the allocation-free clear path, so tick allocates nothing; a drain
// per stage would allocate a large slice per tick only to discard it. Every ring is pre-filled so the "has data" path
// runs (the empty path returns early).
func TestTickNoTapsZeroDrainAllocs(t *testing.T) {
	svc := NewService(Config{
		MaxWindowMs: 500,
		SampleRate:  44100,
	})
	// The constructor stores -1 (the atomic zero is 0); fail loudly if that changes.
	if svc.TapA() != -1 || svc.TapB() != -1 {
		t.Fatalf("expected no taps set; got TapA=%d TapB=%d", svc.TapA(), svc.TapB())
	}

	// A realistic audio-thread push, kept small for speed.
	samples := make([]float64, 4096)
	for i := range samples {
		samples[i] = float64(i)
	}
	pushAll := func() {
		for stage := Stage(0); stage < stageCount; stage++ {
			svc.PushSamples(stage, channel.Of(roster.Kick), samples)
		}
	}
	pushAll()

	// The first ticks may pay one-time costs.
	for i := 0; i < 5; i++ {
		svc.tick()
		pushAll()
	}

	const budget = 0.0
	allocs := testing.AllocsPerRun(50, func() {
		// Refilled before each measured tick so the data path runs; PushSamples settles after warm-up, so any later alloc
		// trips the budget.
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

// With one tap: the tapped stage's drain slice and the published State; the five other stages still clear for free.
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
			svc.PushSamples(stage, channel.Of(roster.Kick), samples)
		}
	}
	pushAll()
	for i := 0; i < 5; i++ {
		svc.tick()
		pushAll()
	}

	// Drain + State = 2 (TapData is a value; State is heap via atomic.Pointer), plus 1 slack for the variadic slice
	// header across Go versions.
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

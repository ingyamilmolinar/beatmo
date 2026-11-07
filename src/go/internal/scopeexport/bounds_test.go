//go:build test

// Opt-in (SCOPE_EXPORT=1). Each stage keeps a ring per instrument id, and drainAll empties rings but keeps keys, so the
// map is bounded by unique ids ever pushed, never by pushes. These pin that bound; a refactor that prunes empty rings or
// adds entries faster than instrument churn fails.

package scopeexport

import (
	"runtime"
	"strconv"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	scope "github.com/ingyamilmolinar/beatmo/internal/scope"
)

// The CURRENT contract: the map never shrinks; adding eviction must update this.
func TestStageRingsMapBoundedByUniqueIDs(t *testing.T) {
	svc := NewService(Config{SampleRate: 44100})

	const uniqueIDs = 200
	samples := []float64{0.1, 0.2, 0.3}

	for i := 0; i < uniqueIDs; i++ {
		id := "inst" + strconv.Itoa(i)
		svc.PushSamples(scope.StageSynth, channel.Of(instrument.ID(id)), samples)
	}

	svc.stages[scope.StageSynth].mu.Lock()
	got := len(svc.stages[scope.StageSynth].rings)
	svc.stages[scope.StageSynth].mu.Unlock()

	if got != uniqueIDs {
		t.Errorf("stages[Synth].rings = %d entries, want %d "+
			"(scopeexport/service.go:90 getOrCreate)", got, uniqueIDs)
	}

	// drainAll keeps keys with empty rings, so the map size is unchanged.
	_ = svc.stages[scope.StageSynth].drainAll()

	svc.stages[scope.StageSynth].mu.Lock()
	gotAfter := len(svc.stages[scope.StageSynth].rings)
	svc.stages[scope.StageSynth].mu.Unlock()

	if gotAfter != uniqueIDs {
		t.Errorf("stages[Synth].rings = %d entries after drainAll, want %d "+
			"(scopeexport/service.go:104 drainAll currently retains keys; "+
			"if you added eviction, update this test and the README)",
			gotAfter, uniqueIDs)
	}
}

// TestStageRingsMapDoesNotGrowOnRepeatedPush asserts that pushing more
// samples for the SAME instrument ID doesn't grow the map past 1 entry.
// This is the "no per-push leak" invariant.
func TestStageRingsMapDoesNotGrowOnRepeatedPush(t *testing.T) {
	svc := NewService(Config{SampleRate: 44100})
	samples := []float64{0.5}

	const repeats = 100_000
	for i := 0; i < repeats; i++ {
		svc.PushSamples(scope.StageSynth, channel.Of(roster.Kick), samples)
	}

	svc.stages[scope.StageSynth].mu.Lock()
	got := len(svc.stages[scope.StageSynth].rings)
	svc.stages[scope.StageSynth].mu.Unlock()

	if got != 1 {
		t.Errorf("stages[Synth].rings = %d entries after %d pushes for the "+
			"same id, want 1 (getOrCreate is creating duplicate rings)",
			got, repeats)
	}
}

// Draining keeps capacity at the largest single push while length returns to 0.
func TestStageRingsBufCapacityBoundedByDrainCadence(t *testing.T) {
	svc := NewService(Config{SampleRate: 44100})
	pushSize := 4096
	buf := make([]float64, pushSize)

	const cycles = 1000
	for c := 0; c < cycles; c++ {
		svc.PushSamples(scope.StageSynth, channel.Of(roster.Kick), buf)
		_ = svc.stages[scope.StageSynth].drainAll()
	}

	rb := svc.stages[scope.StageSynth].rings[channel.Of(roster.Kick)]
	rb.mu.Lock()
	gotLen := len(rb.buf)
	gotCap := cap(rb.buf)
	rb.mu.Unlock()

	if gotLen != 0 {
		t.Errorf("ringBuf.buf len = %d after %d drained cycles, want 0 "+
			"(scopeexport/service.go:80 r.buf = r.buf[:0] regression)",
			gotLen, cycles)
	}
	if gotCap > 2*pushSize {
		t.Errorf("ringBuf.buf cap = %d after %d cycles, want <= %d "+
			"(per-push append-grow leaking; drain not reclaiming "+
			"backing array)", gotCap, cycles, 2*pushSize)
	}
}

// A long soak with a fixed instrument set: no monotonic heap growth.
func TestServiceMemoryFootprintConstantUnderSteadyPlayback(t *testing.T) {
	svc := NewService(Config{SampleRate: 44100})
	samples := make([]float64, 1024)

	// 6 instruments × every stage × tick — matches the real-world fan-out
	// for a 6-row drum machine.
	ids := []string{string(roster.Kick), string(roster.Snare), "hat1", "hat2", "perc1", "perc2"}

	// Warm-up.
	for warm := 0; warm < 32; warm++ {
		for _, id := range ids {
			for st := scope.Stage(0); int(st) < len(svc.stages); st++ {
				svc.PushSamples(st, channel.Of(instrument.ID(id)), samples)
			}
		}
		for st := scope.Stage(0); int(st) < len(svc.stages); st++ {
			_ = svc.stages[st].drainAll()
		}
	}

	runtime.GC()
	var pre runtime.MemStats
	runtime.ReadMemStats(&pre)

	const ticks = 20_000
	for c := 0; c < ticks; c++ {
		for _, id := range ids {
			for st := scope.Stage(0); int(st) < len(svc.stages); st++ {
				svc.PushSamples(st, channel.Of(instrument.ID(id)), samples)
			}
		}
		for st := scope.Stage(0); int(st) < len(svc.stages); st++ {
			_ = svc.stages[st].drainAll()
		}
	}

	runtime.GC()
	var post runtime.MemStats
	runtime.ReadMemStats(&post)

	var delta int64
	if post.HeapAlloc > pre.HeapAlloc {
		delta = int64(post.HeapAlloc - pre.HeapAlloc)
	} else {
		delta = -int64(pre.HeapAlloc - post.HeapAlloc)
	}
	t.Logf("scopeexport HeapAlloc delta after %d ticks = %d bytes "+
		"(ids=%d, fixed)", ticks, delta, len(ids))

	const ceiling = 4 * 1024 * 1024
	if delta > ceiling {
		t.Errorf("scopeexport HeapAlloc delta = %d bytes after %d ticks "+
			"with fixed instrument set (ceiling %d) — service is "+
			"accumulating state proportional to elapsed time even with "+
			"a stable id set", delta, ticks, ceiling)
	}
}

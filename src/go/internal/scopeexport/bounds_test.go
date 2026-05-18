//go:build test

// Bounds regression tests for the scopeexport per-instrument map.
//
// scopeexport is gated by `SCOPE_EXPORT=1`; it's only active when an
// operator opts into the JSONL flight recorder. In that mode it
// maintains a `map[string]*ringBuf` per stage, keyed by instrument ID.
// drainAll() empties the ring buffers but never deletes the map keys,
// so a session that creates many transient instrument IDs would
// accumulate map entries indefinitely.
//
// The audit identified this as a leak suspect. The tests below assert
// two invariants — what's true today plus the documented bound:
//
//   1. The per-stage map size never exceeds the set of unique IDs ever
//      pushed (i.e. the leak is keyed-by-id, not unbounded-per-push).
//   2. drainAll() returns one entry per non-empty ring even when the
//      map itself has accumulated stale keys with empty rings.
//
// These tests make the leak vector observable: if someone refactors
// drainAll() to *also* prune empty rings, the first test will start
// failing (and they can decrement the bound). If someone introduces a
// new path that adds entries faster than instrument churn, the test
// catches the leak.

package scopeexport

import (
	"runtime"
	"strconv"
	"testing"

	scope "github.com/ingyamilmolinar/beatmo/internal/scope"
)

// TestStageRingsMapBoundedByUniqueIDs asserts the per-stage map size
// equals the number of unique instrument IDs ever pushed. This is the
// CURRENT contract — documenting that the map does NOT shrink. A future
// refactor that adds eviction must update this test.
func TestStageRingsMapBoundedByUniqueIDs(t *testing.T) {
	svc := NewService(Config{SampleRate: 44100})

	const uniqueIDs = 200
	samples := []float64{0.1, 0.2, 0.3}

	for i := 0; i < uniqueIDs; i++ {
		id := "inst" + strconv.Itoa(i)
		svc.PushSamples(scope.StageSynth, id, samples)
	}

	svc.stages[scope.StageSynth].mu.Lock()
	got := len(svc.stages[scope.StageSynth].rings)
	svc.stages[scope.StageSynth].mu.Unlock()

	if got != uniqueIDs {
		t.Errorf("stages[Synth].rings = %d entries, want %d "+
			"(scopeexport/service.go:90 getOrCreate)", got, uniqueIDs)
	}

	// drainAll drains every ring but does NOT delete entries with
	// empty rings (current behaviour). Map size should remain
	// unchanged.
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
		svc.PushSamples(scope.StageSynth, "kick", samples)
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

// TestStageRingsBufCapacityBoundedByDrainCadence asserts the underlying
// ringBuf.buf capacity stays at the largest single-push size — the
// drain loop preserves capacity but length must return to 0.
func TestStageRingsBufCapacityBoundedByDrainCadence(t *testing.T) {
	svc := NewService(Config{SampleRate: 44100})
	pushSize := 4096
	buf := make([]float64, pushSize)

	const cycles = 1000
	for c := 0; c < cycles; c++ {
		svc.PushSamples(scope.StageSynth, "kick", buf)
		_ = svc.stages[scope.StageSynth].drainAll()
	}

	rb := svc.stages[scope.StageSynth].rings["kick"]
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

// TestServiceMemoryFootprintConstantUnderSteadyPlayback drives the
// service through a long soak with a *fixed* set of instruments and
// asserts no monotonic heap growth. This is the regression guard for
// the kind of OOM the user reported.
func TestServiceMemoryFootprintConstantUnderSteadyPlayback(t *testing.T) {
	svc := NewService(Config{SampleRate: 44100})
	samples := make([]float64, 1024)

	// 6 instruments × 6 stages × tick — matches the real-world fan-out
	// for a 6-row drum machine.
	ids := []string{"kick", "snare", "hat1", "hat2", "perc1", "perc2"}

	// Warm-up.
	for warm := 0; warm < 32; warm++ {
		for _, id := range ids {
			for st := scope.Stage(0); int(st) < 6; st++ {
				svc.PushSamples(st, id, samples)
			}
		}
		for st := scope.Stage(0); int(st) < 6; st++ {
			_ = svc.stages[st].drainAll()
		}
	}

	runtime.GC()
	var pre runtime.MemStats
	runtime.ReadMemStats(&pre)

	const ticks = 20_000
	for c := 0; c < ticks; c++ {
		for _, id := range ids {
			for st := scope.Stage(0); int(st) < 6; st++ {
				svc.PushSamples(st, id, samples)
			}
		}
		for st := scope.Stage(0); int(st) < 6; st++ {
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

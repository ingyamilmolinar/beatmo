//go:build test

// Bounds regression tests for the scope service ring buffers.
//
// The user's long-session OOM had the Chain (scope) panel active for
// ~9:35 minutes (Beat 1151). Even when no leak has been observed in the
// production scope service, the per-stage ringBuf is fed from the audio
// thread at sample rate — a regression that drops the per-tick drain or
// loosens the `maxSamples` cap would silently turn this into the next
// OOM vector.
//
// These tests assert two invariants:
//   1. Per-stage ringBuf.buf length stays bounded by maxSamples after a
//      sustained push workload + tick() drain cycle.
//   2. Per-stage ringBuf.buf capacity does not grow monotonically over
//      sustained small pushes — i.e. the cap reaches a "high water mark"
//      proportional to the largest single push and stays there.
//
// The tests drive tick() directly so the goroutine scheduler isn't a
// timing variable.

package scope

import (
	"runtime"
	"testing"
)

// TestRingBufBoundedUnderSustainedPushes verifies that draining inside
// tick() keeps each ringBuf.buf length at zero between ticks, and that
// capacity stays bounded by the largest single push size — never grows
// proportional to total samples pushed.
func TestRingBufBoundedUnderSustainedPushes(t *testing.T) {
	svc := NewService(Config{MaxWindowMs: 100, SampleRate: 44100})
	svc.SetTapA(StageSynth)
	svc.SetTapB(StageMaster)

	maxSamples := svc.maxSamples()
	if maxSamples <= 0 {
		t.Fatalf("maxSamples = %d, want positive", maxSamples)
	}

	// Single push slightly larger than the per-tick cap. Capacity will
	// settle at this high-water mark.
	pushSize := maxSamples * 2
	buf := make([]float64, pushSize)
	for i := range buf {
		buf[i] = 0.1
	}

	// Warm-up: a handful of push+drain cycles so capacities reach
	// steady state.
	for warm := 0; warm < 8; warm++ {
		for s := Stage(0); int(s) < int(stageCount); s++ {
			svc.PushSamples(s, "kick", buf)
		}
		svc.tick()
	}

	// Record the high-water mark caps.
	highWaterCap := make([]int, stageCount)
	for s := Stage(0); int(s) < int(stageCount); s++ {
		svc.rings[s].mu.Lock()
		highWaterCap[s] = cap(svc.rings[s].buf)
		svc.rings[s].mu.Unlock()
	}

	// Run another 1000 push+drain cycles. After warm-up the slice
	// underneath the ringBuf has been allocated to fit `pushSize`
	// samples; tick() truncates length to 0 each cycle, so cap should
	// not grow.
	const cycles = 1000
	for c := 0; c < cycles; c++ {
		for s := Stage(0); int(s) < int(stageCount); s++ {
			svc.PushSamples(s, "kick", buf)
		}
		svc.tick()
	}

	// Assertion 1: After every drain, length must be 0 (drain sets
	// r.buf = r.buf[:0]).
	// Assertion 2: cap stays at the high-water mark — append-grow only
	// happens once; subsequent pushes reuse the same backing array.
	for s := Stage(0); int(s) < int(stageCount); s++ {
		svc.rings[s].mu.Lock()
		gotLen := len(svc.rings[s].buf)
		gotCap := cap(svc.rings[s].buf)
		svc.rings[s].mu.Unlock()

		if gotLen != 0 {
			t.Errorf("stage %d: ringBuf.buf len = %d after %d drained cycles, want 0 "+
				"(scope/service.go:46 r.buf = r.buf[:0] regression)",
				s, gotLen, cycles)
		}
		// Cap may grow once on push then a bit more for amortized
		// append, but should not exceed 2× the high-water cap. The 2×
		// tolerance covers the single amortization-doubling that
		// append() may perform when ranging across pushSize+1
		// samples; sustained growth past that is a leak.
		if gotCap > 2*highWaterCap[s] {
			t.Errorf("stage %d: ringBuf.buf cap grew from %d to %d after %d cycles "+
				"(scope/service.go:21 append-grow leaking; per-tick drain not "+
				"reclaiming capacity, OR maxSamples cap loosened)",
				s, highWaterCap[s], gotCap, cycles)
		}
	}
}

// TestRingBufHighWaterTrackedByMaxSamples documents the upper bound on
// the underlying buf capacity: it can never exceed the per-push size
// (which is itself bounded by the audio block size × ticks-since-drain).
// Failing this means the per-stage memory footprint is no longer
// O(window × sample-rate) but O(time elapsed × sample-rate).
func TestRingBufHighWaterTrackedByMaxSamples(t *testing.T) {
	svc := NewService(Config{MaxWindowMs: 500, SampleRate: 44100})
	svc.SetTapA(StageSynth)
	maxSamples := svc.maxSamples() // 22050

	// Push exactly maxSamples once.
	pushBuf := make([]float64, maxSamples)
	svc.PushSamples(StageSynth, "kick", pushBuf)
	svc.tick()

	svc.rings[StageSynth].mu.Lock()
	capAfter := cap(svc.rings[StageSynth].buf)
	svc.rings[StageSynth].mu.Unlock()

	// The cap should be exactly maxSamples (single append that filled
	// the empty slice). Allow Go's append() to round up by a single
	// growth step (typically 2×) — anything past that means tick()
	// failed to truncate.
	if capAfter > 2*maxSamples {
		t.Errorf("ringBuf.buf cap = %d after single push of %d samples, "+
			"want ≤ %d (single-allocation high-water mark)",
			capAfter, maxSamples, 2*maxSamples)
	}
}

// TestRingBufMemoryFootprintConstantUnderSteadyPlayback runs a longer
// soak (mirroring the user's 9:35-minute Chain panel session in
// compressed form) and asserts the process HeapAlloc delta is small —
// the scope subsystem alone must not allocate proportionally to elapsed
// time.
func TestRingBufMemoryFootprintConstantUnderSteadyPlayback(t *testing.T) {
	svc := NewService(Config{MaxWindowMs: 100, SampleRate: 44100})
	svc.SetTapA(StageSynth)
	svc.SetTapB(StageMaster)

	// 1024 samples per push × 6 stages × tick — mirrors what the audio
	// thread pushes per 33 ms block.
	pushBuf := make([]float64, 1024)

	// Warm up so any one-shot allocations are accounted for.
	for warm := 0; warm < 32; warm++ {
		for s := Stage(0); int(s) < int(stageCount); s++ {
			svc.PushSamples(s, "kick", pushBuf)
		}
		svc.tick()
	}

	runtime.GC()
	var pre runtime.MemStats
	runtime.ReadMemStats(&pre)

	// 60000 ticks ≈ 33 min of simulated playback at 33 ms tick.
	const ticks = 60_000
	for c := 0; c < ticks; c++ {
		for s := Stage(0); int(s) < int(stageCount); s++ {
			svc.PushSamples(s, "kick", pushBuf)
		}
		svc.tick()
	}

	runtime.GC()
	var post runtime.MemStats
	runtime.ReadMemStats(&post)

	// HeapAlloc delta should be near zero — scope service must reach
	// steady state. tick() *does* allocate (the drain returns a copy
	// in `make([]float64, len(src))`) but those allocations are
	// transient and reclaimed by GC. We only check the resident
	// delta, not TotalAlloc.
	var delta int64
	if post.HeapAlloc > pre.HeapAlloc {
		delta = int64(post.HeapAlloc - pre.HeapAlloc)
	} else {
		delta = -int64(pre.HeapAlloc - post.HeapAlloc)
	}
	t.Logf("HeapAlloc delta after %d ticks = %d bytes", ticks, delta)

	// 4 MiB ceiling. The scope service's resident footprint should
	// be ~1 MiB (6 stages × 22k samples × 8 bytes). Anything past
	// 4 MiB delta means a structure is accumulating proportional to
	// elapsed time — that's the OOM vector we're guarding against.
	const ceiling = 4 * 1024 * 1024
	if delta > ceiling {
		t.Errorf("scope service HeapAlloc delta = %d bytes after %d ticks "+
			"(ceiling %d); the per-stage ringBuf or some derived structure "+
			"is leaking — check service.go tick() and rings[i].drain() for "+
			"a path that retains samples beyond the per-tick window",
			delta, ticks, ceiling)
	}
}

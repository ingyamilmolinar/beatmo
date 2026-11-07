//go:build test

// The per-stage ring is fed from the audio thread at sample rate, so dropping the per-tick drain or loosening the
// maxSamples cap would be an OOM vector over a long session. Length stays within maxSamples and capacity at a high-water
// mark; tick() is driven directly so scheduling is no variable.

package scope

import (
	"runtime"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Capacity stays bounded by the largest single push, never by total samples pushed.
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

	// Capacities reach steady state first.
	for warm := 0; warm < 8; warm++ {
		for s := Stage(0); int(s) < int(stageCount); s++ {
			svc.PushSamples(s, channel.Of(roster.Kick), buf)
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

	// tick() truncates to 0 each cycle, so after warm-up the capacity must not grow.
	const cycles = 1000
	for c := 0; c < cycles; c++ {
		for s := Stage(0); int(s) < int(stageCount); s++ {
			svc.PushSamples(s, channel.Of(roster.Kick), buf)
		}
		svc.tick()
	}

	// Length 0 after every drain, capacity at the high-water mark (append grows once, then reuses).
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
		// 2× covers append's one amortised doubling; sustained growth past it is a leak.
		if gotCap > 2*highWaterCap[s] {
			t.Errorf("stage %d: ringBuf.buf cap grew from %d to %d after %d cycles "+
				"(scope/service.go:21 append-grow leaking; per-tick drain not "+
				"reclaiming capacity, OR maxSamples cap loosened)",
				s, highWaterCap[s], gotCap, cycles)
		}
	}
}

// Capacity never exceeds the per-push size, or the footprint is O(elapsed time), not O(window).
func TestRingBufHighWaterTrackedByMaxSamples(t *testing.T) {
	svc := NewService(Config{MaxWindowMs: 500, SampleRate: 44100})
	svc.SetTapA(StageSynth)
	maxSamples := svc.maxSamples() // 22050

	// Push exactly maxSamples once.
	pushBuf := make([]float64, maxSamples)
	svc.PushSamples(StageSynth, channel.Of(roster.Kick), pushBuf)
	svc.tick()

	svc.rings[StageSynth].mu.Lock()
	capAfter := cap(svc.rings[StageSynth].buf)
	svc.rings[StageSynth].mu.Unlock()

	// One append growth step allowed; past it, tick() failed to truncate.
	if capAfter > 2*maxSamples {
		t.Errorf("ringBuf.buf cap = %d after single push of %d samples, "+
			"want ≤ %d (single-allocation high-water mark)",
			capAfter, maxSamples, 2*maxSamples)
	}
}

// A compressed long Chain-panel session: the scope subsystem must not allocate in proportion to elapsed time.
func TestRingBufMemoryFootprintConstantUnderSteadyPlayback(t *testing.T) {
	svc := NewService(Config{MaxWindowMs: 100, SampleRate: 44100})
	svc.SetTapA(StageSynth)
	svc.SetTapB(StageMaster)

	// What the audio thread pushes per block across six stages.
	pushBuf := make([]float64, 1024)

	// Warm up so any one-shot allocations are accounted for.
	for warm := 0; warm < 32; warm++ {
		for s := Stage(0); int(s) < int(stageCount); s++ {
			svc.PushSamples(s, channel.Of(roster.Kick), pushBuf)
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
			svc.PushSamples(s, channel.Of(roster.Kick), pushBuf)
		}
		svc.tick()
	}

	runtime.GC()
	var post runtime.MemStats
	runtime.ReadMemStats(&post)

	// tick's drain copies are transient and reclaimed, so only the resident delta is checked, not TotalAlloc.
	var delta int64
	if post.HeapAlloc > pre.HeapAlloc {
		delta = int64(post.HeapAlloc - pre.HeapAlloc)
	} else {
		delta = -int64(pre.HeapAlloc - post.HeapAlloc)
	}
	t.Logf("HeapAlloc delta after %d ticks = %d bytes", ticks, delta)

	// The resident footprint is ~1 MiB (6 stages × 22k samples × 8 B); past 4 MiB something accumulates with time.
	const ceiling = 4 * 1024 * 1024
	if delta > ceiling {
		t.Errorf("scope service HeapAlloc delta = %d bytes after %d ticks "+
			"(ceiling %d); the per-stage ringBuf or some derived structure "+
			"is leaking — check service.go tick() and rings[i].drain() for "+
			"a path that retains samples beyond the per-tick window",
			delta, ticks, ceiling)
	}
}

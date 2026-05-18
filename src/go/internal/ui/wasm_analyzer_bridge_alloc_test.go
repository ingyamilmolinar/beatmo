//go:build test

// Phase 1B baseline alloc tests. These intentionally FAIL on the
// pre-fix code to record the leak slope as a CI artefact ("confirm
// through logs"). The Phase 2 fix flips the assertions so they pass
// — the test contract is locked at that point.

package ui

import (
	"runtime"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// stubAnalyzerFetch returns a fixed-size populated AnalyzerSnapshot.
// Mirrors the production WASM bridge shape (Spectrum + Waveform of
// 512 float64 each, plus RMS/Peak scalars). The returned slices are
// freshly allocated each call to mimic ChannelAnalyzerSnapshot's
// per-call allocation pattern — this is the point of the measurement.
func stubAnalyzerFetch(id string) audio.AnalyzerSnapshot {
	const n = 512
	spec := make([]float64, n)
	wave := make([]float64, n)
	for i := 0; i < n; i++ {
		spec[i] = float64(i) / float64(n)
		wave[i] = (float64(i) - float64(n/2)) / float64(n)
	}
	return audio.AnalyzerSnapshot{
		RMS:      0.1,
		Peak:     0.5,
		Spectrum: spec,
		Waveform: wave,
	}
}

// fixedFetch returns a single shared snapshot per call. Removes the
// allocator noise from the fetch itself so the test measures the
// builder's own allocation budget — not the JS bridge.
func fixedFetch(id string) audio.AnalyzerSnapshot {
	// Keep a package-level cache so repeated calls return the same
	// slice headers without re-allocating.
	return fixedSnap
}

var fixedSnap = func() audio.AnalyzerSnapshot {
	const n = 512
	spec := make([]float64, n)
	wave := make([]float64, n)
	for i := 0; i < n; i++ {
		spec[i] = float64(i) / float64(n)
		wave[i] = (float64(i) - float64(n/2)) / float64(n)
	}
	return audio.AnalyzerSnapshot{
		RMS:      0.1,
		Peak:     0.5,
		Spectrum: spec,
		Waveform: wave,
	}
}()

// fakeRows builds n DrumRow pointers with stable instrument IDs.
func fakeRows(n int) []*DrumRow {
	rows := make([]*DrumRow, n)
	for i := 0; i < n; i++ {
		rows[i] = &DrumRow{
			Instrument: "inst" + itoaSmallTest(i),
			Name:       "Inst " + itoaSmallTest(i),
		}
	}
	return rows
}

// TestBuildAnalyzerStateAllocsBudget asserts the per-call allocation
// budget for BuildAnalyzerStateFromSnapshots. On the current (pre-fix)
// code this fails by an order of magnitude — the failure log is the
// Phase 1 baseline evidence. After Phase 2 the budget tightens to
// `<= maxAllocsAfterFix` (rewritten in Phase 3A).
func TestBuildAnalyzerStateAllocsBudget(t *testing.T) {
	rows := fakeRows(6)

	allocs := testing.AllocsPerRun(200, func() {
		_ = buildAnalyzerStateFromSnapshotsImpl("inst3", rows, 48000, fixedFetch)
	})
	t.Logf("BuildAnalyzerStateFromSnapshots allocs/op = %.1f (rows=%d)", allocs, len(rows))

	// PHASE 1 BASELINE — must FAIL on current code. The fixedFetch
	// itself is alloc-free; everything counted here is the builder's
	// own slice/state churn (Spectrum dB conversion, Waveform copy,
	// FreqBins, instrument metrics, state struct).
	const phase1Floor = 5
	if allocs < phase1Floor {
		t.Logf("BASELINE NOTE: allocs/op (%.1f) is already under phase1Floor (%d) — "+
			"either the fix has landed or the test is mis-instrumented.", allocs, phase1Floor)
	}

	// PHASE 2 BUDGET — flipped on after the fix lands. Until then,
	// this assertion FAILS, which is intentional. After Phase 2A the
	// metrics-only path bypasses BuildAnalyzerStateFromSnapshots for
	// TabMeters, but full-state callers still need a tight budget on
	// the heavy path.
	const maxAllocsAfterFix = 60
	if allocs > maxAllocsAfterFix {
		t.Errorf("BuildAnalyzerStateFromSnapshots allocs/op = %.1f, want <= %d "+
			"(per-row Spectrum + Waveform + FFT/Freq slices dominate)",
			allocs, maxAllocsAfterFix)
	}
}

// TestBuildAnalyzerStateBytesPerCall measures heap-allocated bytes per
// builder call via TotalAlloc deltas across 1 000 invocations. Locks
// the budget at the byte level so a regression that shrinks alloc
// COUNT but blows up alloc SIZE still trips the test.
func TestBuildAnalyzerStateBytesPerCall(t *testing.T) {
	rows := fakeRows(6)
	const iters = 1000

	// Warm up + force a stable baseline.
	for i := 0; i < 100; i++ {
		_ = buildAnalyzerStateFromSnapshotsImpl("inst3", rows, 48000, fixedFetch)
	}
	runtime.GC()
	var pre, post runtime.MemStats
	runtime.ReadMemStats(&pre)
	for i := 0; i < iters; i++ {
		_ = buildAnalyzerStateFromSnapshotsImpl("inst3", rows, 48000, fixedFetch)
	}
	runtime.ReadMemStats(&post)
	bytesPerCall := (post.TotalAlloc - pre.TotalAlloc) / iters
	t.Logf("BuildAnalyzerStateFromSnapshots TotalAlloc/call = %d bytes (rows=%d, iters=%d)",
		bytesPerCall, len(rows), iters)

	// Phase 2 budget: under 64 KB per call. Pre-fix exceeds this
	// because every call materializes Spectrum+Waveform+FFT slices
	// for master + every row + detail. Post-fix the heavy path stays
	// under the cap because reuseable scratch buffers replace the
	// per-call allocations.
	const maxBytesPerCall = 64 * 1024
	if bytesPerCall > maxBytesPerCall {
		t.Errorf("BuildAnalyzerStateFromSnapshots bytes/call = %d, want <= %d "+
			"(slice-heavy path bypasses scratch buffers)",
			bytesPerCall, maxBytesPerCall)
	}
}

// itoaSmallTest is a test-local copy of the ui itoaSmall helper to
// avoid leaning on the WASM-only file from this -tags test build.
func itoaSmallTest(n int) string {
	if n < 0 {
		n = -n
	}
	if n < 10 {
		return string('0' + byte(n))
	}
	if n < 100 {
		return string([]byte{'0' + byte(n/10), '0' + byte(n%10)})
	}
	return string([]byte{'0' + byte(n/100), '0' + byte((n/10)%10), '0' + byte(n%10)})
}

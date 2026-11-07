//go:build test

package ui

import (
	"runtime"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// Sized from the bridge constants, so the budget measures what the bridge actually ships. Fresh slices on every
// call, as ChannelAnalyzerSnapshot allocates.
func stubAnalyzerFetch(id channel.ID) audio.AnalyzerSnapshot {
	spec := make([]float64, audio.BridgeSpectrumBins)
	wave := make([]float64, audio.BridgeWaveformSamples)
	for i := range spec {
		spec[i] = float64(i) / float64(len(spec))
	}
	for i := range wave {
		wave[i] = (float64(i) - float64(len(wave)/2)) / float64(len(wave))
	}
	return audio.AnalyzerSnapshot{
		RMS:      0.1,
		Peak:     0.5,
		Spectrum: spec,
		Waveform: wave,
	}
}

// One shared snapshot, so only the builder's own allocations are counted.
func fixedFetch(id channel.ID) audio.AnalyzerSnapshot {
	return fixedSnap
}

var fixedSnap = func() audio.AnalyzerSnapshot {
	spec := make([]float64, audio.BridgeSpectrumBins)
	wave := make([]float64, audio.BridgeWaveformSamples)
	for i := range spec {
		spec[i] = float64(i) / float64(len(spec))
	}
	for i := range wave {
		wave[i] = (float64(i) - float64(len(wave)/2)) / float64(len(wave))
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
			Instrument: instrument.ID("inst" + itoaSmallTest(i)),
			Name:       "Inst " + itoaSmallTest(i),
		}
	}
	return rows
}

func TestBuildAnalyzerStateAllocsBudget(t *testing.T) {
	rows := fakeRows(6)

	allocs := testing.AllocsPerRun(200, func() {
		_ = buildAnalyzerStateFromSnapshotsImpl(channel.Of("inst3"), rows, 48000, fixedFetch, nil)
	})
	t.Logf("BuildAnalyzerStateFromSnapshots allocs/op = %.1f (rows=%d)", allocs, len(rows))

	// fixedFetch allocates nothing: everything counted is the builder's own churn. Under the floor means the
	// instrumentation broke.
	const phase1Floor = 5
	if allocs < phase1Floor {
		t.Logf("BASELINE NOTE: allocs/op (%.1f) is already under phase1Floor (%d) — "+
			"either the fix has landed or the test is mis-instrumented.", allocs, phase1Floor)
	}

	// Full-state callers still need a tight budget on the heavy path.
	const maxAllocsAfterFix = 60
	if allocs > maxAllocsAfterFix {
		t.Errorf("BuildAnalyzerStateFromSnapshots allocs/op = %.1f, want <= %d "+
			"(per-row Spectrum + Waveform + FFT/Freq slices dominate)",
			allocs, maxAllocsAfterFix)
	}
}

// Bytes as well as count: fewer but larger allocations still trip it.
func TestBuildAnalyzerStateBytesPerCall(t *testing.T) {
	rows := fakeRows(6)
	const iters = 1000

	// Warm up + force a stable baseline.
	for i := 0; i < 100; i++ {
		_ = buildAnalyzerStateFromSnapshotsImpl(channel.Of("inst3"), rows, 48000, fixedFetch, nil)
	}
	runtime.GC()
	var pre, post runtime.MemStats
	runtime.ReadMemStats(&pre)
	for i := 0; i < iters; i++ {
		_ = buildAnalyzerStateFromSnapshotsImpl(channel.Of("inst3"), rows, 48000, fixedFetch, nil)
	}
	runtime.ReadMemStats(&post)
	bytesPerCall := (post.TotalAlloc - pre.TotalAlloc) / iters
	t.Logf("BuildAnalyzerStateFromSnapshots TotalAlloc/call = %d bytes (rows=%d, iters=%d)",
		bytesPerCall, len(rows), iters)

	// Reused scratch buffers keep the heavy path under it.
	const maxBytesPerCall = 64 * 1024
	if bytesPerCall > maxBytesPerCall {
		t.Errorf("BuildAnalyzerStateFromSnapshots bytes/call = %d, want <= %d "+
			"(slice-heavy path bypasses scratch buffers)",
			bytesPerCall, maxBytesPerCall)
	}
}

// The original lives in a WASM-only file.
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

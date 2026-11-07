//go:build test

// Minutes of steady playback with the Meters tab open, running Update and the EQ-panel Draw every frame, so the
// analyzer-snapshot bridge runs on production's schedule.

package ui

import (
	"runtime"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// Rounded down to keep the run under a CI minute.
const metersCanaryFrames = 60_000

// Enough samples for a slope without dominating the cost.
const metersCanarySampleEvery = 5_000

// The SLOPE is the load-bearing check: a steady leak gives a positive slope that an absolute cap misses
// when GC happens to run just before the last sample. The bridge read ratio is 0 under -tags test (stubbed).
func TestLongSessionMetersTabHeapBounded(t *testing.T) {
	if testing.Short() {
		t.Skip("long canary skipped under -short; run via nightly")
	}
	withDefaultAudio(t)
	withDefaultStart(t, false)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	g.SetPlayFunc(func(instrument.ID, float64, ...float64) {})
	if err := g.SetSubdivisions(16); err != nil {
		t.Fatalf("set subdivisions: %v", err)
	}
	g.drum.SetBPM(240)
	buildSoakScene(t, g, 6 /* rows */, 8 /* nodes per row */)

	// The WASM stub returns nil state, so an override routes through the real builder with the bridge's per-call
	// slice churn.
	if g.drum.eqPanelZone == nil {
		t.Fatal("eqPanelZone nil; harness assumption broken")
	}
	g.drum.eqPanelZone.tabState.SetActiveTab(TabMeters)
	// The heavy per-row path: a regression routing Meters back through it blows the heap cap.
	g.drum.eqPanelZone.callbacks.AnalyzerState = func() *analyzer.State {
		return buildAnalyzerStateFromSnapshotsImpl(channel.ID{}, g.drum.Rows, 48000, allocFetch, nil)
	}
	// The scalar path Meters must take; otherwise the heavy callback above fires every frame.
	g.drum.eqPanelZone.callbacks.AnalyzerMetricsOnly = func() *analyzer.State {
		return buildAnalyzerMetricsOnlyImpl(g.drum.Rows, scalarFetch)
	}

	scratch := ebiten.NewImage(1280, 720)
	g.SetPlaying(true)
	advanceFrames(g, 5)

	audio.ResetAnalyzerBridgeStats()

	var pre runtime.MemStats
	runtime.GC()
	runtime.GC()
	runtime.ReadMemStats(&pre)

	var samples []memSample
	for i := 1; i <= metersCanaryFrames; i++ {
		setPlayStartForAbs(g, i)
		g.engine.Predictor.Ensure(i + 1)
		_ = g.Update()
		g.drum.eqPanelZone.Draw(scratch)

		if i%metersCanarySampleEvery == 0 {
			var ms runtime.MemStats
			runtime.ReadMemStats(&ms)
			samples = append(samples, memSample{frame: i, heapAlloc: ms.HeapAlloc, totalAlloc: ms.TotalAlloc})
		}
	}

	var post runtime.MemStats
	runtime.GC()
	runtime.GC()
	runtime.ReadMemStats(&post)

	heapAllocDelta := int64(post.HeapAlloc) - int64(pre.HeapAlloc)
	totalAllocDelta := post.TotalAlloc - pre.TotalAlloc
	bridgeCalls, bridgeReads := audio.AnalyzerBridgeStats()

	// Logged always, so a CI run leaves a series to grep.
	t.Logf("=== Meters-tab canary memstats slope (frames=%d) ===", metersCanaryFrames)
	for _, s := range samples {
		t.Logf("  frame=%6d heapAlloc=%6d KB totalAlloc=%9d KB",
			s.frame, s.heapAlloc>>10, s.totalAlloc>>10)
	}
	t.Logf("HeapAlloc delta = %d KB (pre=%d KB post=%d KB)",
		heapAllocDelta>>10, pre.HeapAlloc>>10, post.HeapAlloc>>10)
	t.Logf("TotalAlloc delta = %d KB (avg %d bytes/frame across %d frames)",
		totalAllocDelta>>10, totalAllocDelta/uint64(metersCanaryFrames), metersCanaryFrames)
	t.Logf("Analyzer bridge stats: calls=%d elementReads=%d (ratio=%.1f reads/call)",
		bridgeCalls, bridgeReads, ratio(bridgeReads, bridgeCalls))

	// Both samples are post-warm-up; a non-zero slope means the loop retains state per frame.
	slopeBytesPerFrame := int64(0)
	if len(samples) >= 2 {
		first := samples[len(samples)/2] // mid-run sample (after warmup)
		last := samples[len(samples)-1]
		spanFrames := int64(last.frame - first.frame)
		if spanFrames > 0 {
			slopeBytesPerFrame = (int64(last.heapAlloc) - int64(first.heapAlloc)) / spanFrames
		}
	}
	t.Logf("Late-window heap slope = %d bytes/frame", slopeBytesPerFrame)

	// Generous: the shared binary carries other tests' residue. It flags catastrophic leaks; the slope below
	// enforces the bound.
	const heapAllocCap = 300 << 20 // 300 MB
	if heapAllocDelta > heapAllocCap {
		t.Errorf("HeapAlloc delta = %d KB after %d Meters-tab frames (cap %d KB) — "+
			"per-frame analyzer-snapshot path is leaking",
			heapAllocDelta>>10, metersCanaryFrames, heapAllocCap>>10)
	}

	// Sized for full-suite runs, where other tests' residue skews the slope; a real per-frame leak still exceeds it.
	const slopeCapBytesPerFrame = 8192
	if slopeBytesPerFrame > slopeCapBytesPerFrame {
		t.Errorf("Late-window heap slope = %d bytes/frame (cap %d) — "+
			"steady-state retention; Update+Draw is not GC-stable",
			slopeBytesPerFrame, slopeCapBytesPerFrame)
	}

	// WASM only: the bridge counters are 0 without the JS bridge.
	const readsPerCallCap = 8
	if bridgeCalls > 0 {
		if got := ratio(bridgeReads, bridgeCalls); got > readsPerCallCap {
			t.Errorf("Analyzer bridge reads/call = %.1f (cap %d) — "+
				"per-element js.Value loop survived; Phase 2C bulk transfer missing",
				got, readsPerCallCap)
		}
	}
}

type memSample struct {
	frame      int
	heapAlloc  uint64
	totalAlloc uint64
}

func ratio(num, den uint64) float64 {
	if den == 0 {
		return 0
	}
	return float64(num) / float64(den)
}

// FRESH slices on every call, as the WASM bridge allocates.
func allocFetch(id channel.ID) audio.AnalyzerSnapshot {
	const n = 512
	spec := make([]float64, n)
	wave := make([]float64, n)
	for i := 0; i < n; i++ {
		spec[i] = float64(i) / float64(n)
		wave[i] = (float64(i) - float64(n/2)) / float64(n)
	}
	return audio.AnalyzerSnapshot{
		RMS:       0.1,
		Peak:      0.5,
		ClipCount: 0,
		Spectrum:  spec,
		Waveform:  wave,
	}
}

func scalarFetch(id channel.ID) (peak, rms float64, clips int, active bool) {
	return 0.5, 0.1, 0, true
}

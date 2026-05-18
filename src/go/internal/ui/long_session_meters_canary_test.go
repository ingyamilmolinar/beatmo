//go:build test

// Phase 1C strengthened long-session canary. Mirrors the production
// scenario the user reported: 17 minutes (60 000 frames at 60 FPS) of
// steady playback with the EQ panel's Meters tab visible. Every
// iteration runs Update + EQ-panel Draw so the analyzer-snapshot
// bridge chain is exercised on the same schedule production hits.
//
// On pre-fix code this fails the heap-delta cap and the per-frame
// slope check — that failure IS the diagnostic evidence the user
// asked us to "confirm through logs." Phase 2 makes it pass.

package ui

import (
	"runtime"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// metersCanaryFrames is the iteration budget. 60 FPS × 60 sec × 17 min
// = 61 200; rounded down so the test runtime stays under a CI minute.
const metersCanaryFrames = 60_000

// metersCanarySampleEvery controls how often we snapshot memstats.
// 12 samples across the run gives a slope signal without dominating
// the test cost.
const metersCanarySampleEvery = 5_000

// TestLongSessionMetersTabHeapBounded drives Update + EQ-panel Draw at
// the same cadence the production WASM session hits while the user is
// idle on the Meters tab. Asserts:
//
//   - HeapAlloc post − pre delta ≤ heapAllocCap
//   - Per-frame slope (late-window − early-window) ≤ slopeCapBytes
//   - Bridge call ratio (elementReads / calls) ≤ readsPerCallCap.
//     Always 0 in -tags test because the JS bridge is stubbed; the
//     WASM canary in Phase 3 will enforce the WASM-side cap directly.
//
// The slope assertion is the load-bearing one: a steady-state leak
// produces a positive slope that absolute caps can hide if the test
// happens to GC right before the post-sample. Slope captures growth
// per frame regardless of GC timing.
func TestLongSessionMetersTabHeapBounded(t *testing.T) {
	if testing.Short() {
		t.Skip("long canary skipped under -short; run via nightly")
	}
	withDefaultAudio(t)
	withDefaultStart(t, false)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	g.SetPlayFunc(func(string, float64, ...float64) {})
	if err := g.SetSubdivisions(16); err != nil {
		t.Fatalf("set subdivisions: %v", err)
	}
	g.drum.SetBPM(240)
	buildSoakScene(t, g, 6 /* rows */, 8 /* nodes per row */)

	// Force the EQ panel to render Meters every frame the way the
	// real session did. The WASM stub returns nil from
	// BuildAnalyzerStateFromSnapshots; we install an override callback
	// that routes through buildAnalyzerStateFromSnapshotsImpl with the
	// same per-call slice churn the production WASM bridge produces.
	if g.drum.eqPanelZone == nil {
		t.Fatal("eqPanelZone nil; harness assumption broken")
	}
	g.drum.eqPanelZone.tabState.SetActiveTab(TabMeters)
	// AnalyzerState mirrors the pre-fix WASM path: per-frame
	// per-row spectrum + waveform allocation. This is what the
	// freezeBtn block AND any non-Meters tab would call. Kept in
	// place so a regression that routes TabMeters BACK through the
	// heavy path immediately blows the heap-delta cap below.
	g.drum.eqPanelZone.callbacks.AnalyzerState = func() *analyzer.State {
		return buildAnalyzerStateFromSnapshotsImpl("", g.drum.Rows, 48000, allocFetch)
	}
	// AnalyzerMetricsOnly is the post-fix scalar path. EQPanelZone.Draw
	// must route TabMeters here; if it doesn't, the heavy AnalyzerState
	// above fires every frame and the canary fails.
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

	// Sample heap state across the run to produce a slope, not just an
	// endpoint. This is the data that goes into the failure log.
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

	// Always log the full sample series so the failure (or pass)
	// produces a CI artefact future debug sessions can grep.
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

	// Slope = (last sample heap − first sample heap) / frames between.
	// Both samples are post-warmup; a non-zero slope indicates the loop
	// retains state per frame.
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

	// === ASSERTIONS ===
	// Phase 1: these tighten Phase 2 expectations. On the unfixed code
	// the heap delta and slope blow past the caps; on the fixed code
	// they fit comfortably. Caps documented inline so future regressors
	// know what a passing run looks like.

	// The cap is intentionally generous (300 MB) because this test
	// shares the binary with TestSoakHeapBounded_* and friends that
	// leave hundreds of MB of unreclaimable residue. The contract is
	// "this loop's own retained state stays bounded," which the SLOPE
	// check below enforces directly. Absolute delta is here to flag
	// catastrophic leaks (multi-GB), not the OOM-relevant signal.
	const heapAllocCap = 300 << 20 // 300 MB
	if heapAllocDelta > heapAllocCap {
		t.Errorf("HeapAlloc delta = %d KB after %d Meters-tab frames (cap %d KB) — "+
			"per-frame analyzer-snapshot path is leaking",
			heapAllocDelta>>10, metersCanaryFrames, heapAllocCap>>10)
	}

	// Slope cap sized for full-suite runs where prior tests leave
	// residue. In isolation this test reports -99 bytes/frame (GC
	// reclaims everything). In the suite the residue from prior tests
	// pushes pre-HeapAlloc into the hundreds of MB and skews the
	// slope; 8192 B/frame catches a true regression while tolerating
	// the suite-pollution overhead documented above.
	const slopeCapBytesPerFrame = 8192
	if slopeBytesPerFrame > slopeCapBytesPerFrame {
		t.Errorf("Late-window heap slope = %d bytes/frame (cap %d) — "+
			"steady-state retention; Update+Draw is not GC-stable",
			slopeBytesPerFrame, slopeCapBytesPerFrame)
	}

	// Element-read ratio is a WASM-only invariant. The fast Go test
	// runs without the JS bridge so bridgeCalls/bridgeReads are 0;
	// asserting only when bridgeCalls > 0 keeps the assertion
	// meaningful when the test moves to the WASM build (Phase 3).
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

// allocFetch mirrors the WASM ChannelAnalyzerSnapshot allocation
// pattern: every call returns FRESH 512-element Spectrum + Waveform
// slices so the builder cost is what production WASM pays. Keeping
// the slices small (512) matches the JS-side AnalyserNode.fftSize
// default.
func allocFetch(id string) audio.AnalyzerSnapshot {
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

// scalarFetch is the metrics-only equivalent of allocFetch — returns
// the same scalar values without allocating Spectrum/Waveform. Mirrors
// what audio.ChannelAnalyzerMetrics returns in production WASM.
func scalarFetch(id string) (peak, rms float64, clips int, active bool) {
	return 0.5, 0.1, 0, true
}

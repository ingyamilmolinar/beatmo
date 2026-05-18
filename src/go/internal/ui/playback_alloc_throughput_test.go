//go:build test

package ui

import (
	"fmt"
	"runtime"
	"sort"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestPlaybackAllocThroughput is the regression test for the fast WASM OOM
// reported on add-core-node-types: ~2.1 GB used after ~13 s of play.
//
// The OOM root cause was per-frame `screen.SubImage(...)` allocations in
// (*DrumViewTree).Draw and (*Game).drawGridPane — each call returned a
// fresh *ebiten.Image wrapper that ebiten internally treated as a new GPU
// resource. At 60 FPS × ~10 zones, this generated ~600 image-wrapper
// allocations per second, totaling ~5.7 MB/frame and OOMing WASM in seconds.
//
// Fix: cache the SubImage wrapper per zone (drumview_tree.go) and per grid
// pane (game_struct.go + game_draw_grid_pane.go). The wrapper is reused
// across frames when the parent screen pointer and clip rectangle are
// unchanged. Reduced throughput by ~498× (5876 KB/frame → 11.8 KB/frame).
//
// If this test fails in the future, look for new per-frame *ebiten.Image
// allocations or other sustained Go-heap allocators in the Update/Draw path.
//
// This test deliberately differs from the existing soak harness in
// soak_heap_bound_test.go in two critical ways:
//
//  1. It calls g.Update() AND g.Draw(screen) every frame. The existing soak
//     calls Draw only every 30 frames (sc.drawEvery default), which under-
//     samples Draw-path allocators by 30×.
//  2. It does NOT force runtime.GC() between samples. The existing soak
//     samples post-GC, so it measures *retained* leaks. WASM's single-
//     threaded GC cannot keep up with the production allocation rate, so the
//     OOM is driven by *throughput* (TotalAlloc/frame) rather than retention.
//     Forcing GC hides the throughput problem.
//
// The bound is per-frame TotalAlloc rate: at 60 FPS, anything above ~80 KB/
// frame projects to >10× WASM linear memory in 30 s of play. The user's OOM
// hit ~2.1 GB in 13 s — that's ~2.7 MB/frame, ~33× the ceiling here. So the
// bound is generous; a real leak overshoots by an order of magnitude.
//
// We also bound the per-frame *image* allocation count (via the
// image_metrics tracker). Steady-state playback should allocate 0 new
// *ebiten.Image instances per frame; a tracked image alloc rate >0 means
// some cache is invalidating per frame and is the most likely OOM culprit.
//
// On RED, the test logs which tracked image tag has the highest alloc rate
// — pointing directly at the leaking allocator.
func TestPlaybackAllocThroughput(t *testing.T) {
	withDefaultAudio(t)
	withDefaultStart(t, false)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	g.SetPlayFunc(func(string, float64, ...float64) {})

	// Default subdivisions/BPM, default circuit (built by the standard test
	// helper). User repro is "vanilla load → press play".
	if err := g.SetSubdivisions(8); err != nil {
		t.Fatalf("set subdiv: %v", err)
	}
	g.drum.SetBPM(120)
	buildSoakScene(t, g, 4 /* rows */, 8 /* nodes per row */)

	screen := ebiten.NewImage(1280, 720)

	// Warm-up: the first ~120 frames legitimately allocate caches (zone
	// layouts, sprite atlases). Measurement window starts after warm-up.
	const warmupFrames = 120
	const measureFrames = 1500

	// Start playback BEFORE warm-up so steady-state is reached during warm-up.
	g.SetPlaying(true)
	advanceFrames(g, 5)
	for i := 0; i < warmupFrames; i++ {
		_ = g.Update()
		g.Draw(screen)
	}

	// Reset image metrics so the measurement window only counts allocations
	// AFTER warm-up.
	ResetImageMetrics()

	var pre runtime.MemStats
	runtime.ReadMemStats(&pre)

	// Drive measureFrames at production rate: Update + Draw every frame.
	// No GC forcing — we want raw allocation throughput.
	for i := 0; i < measureFrames; i++ {
		_ = g.Update()
		g.Draw(screen)
	}

	var post runtime.MemStats
	runtime.ReadMemStats(&post)

	totalAllocDelta := post.TotalAlloc - pre.TotalAlloc
	imageAllocsDelta := MetricImagesAllocatedTotal()
	bytesPerFrame := float64(totalAllocDelta) / float64(measureFrames)
	imagesPerFrame := float64(imageAllocsDelta) / float64(measureFrames)

	// Per-frame TotalAlloc bound. 80 KB/frame = ~5 MB/s @ 60 FPS, well within
	// what WASM's GC can sustain. The user's OOM rate was ~2.7 MB/frame.
	const bytesPerFrameBound = 80 * 1024

	// Per-frame image-alloc bound. Steady-state = 0 new images. We allow a
	// hard cap of 0.05 (= 1 image per 20 frames) for legitimate occasional
	// resize handling; the leak under investigation is several per frame.
	const imagesPerFrameBound = 0.05

	failed := false
	if bytesPerFrame > bytesPerFrameBound {
		failed = true
		t.Errorf("TotalAlloc throughput=%.1f KB/frame over %d frames (bound=%.0f KB/frame). "+
			"At 60 FPS this is %.2f MB/s — projected to 2 GB WASM ceiling in ~%.0f s.",
			bytesPerFrame/1024, measureFrames, float64(bytesPerFrameBound)/1024,
			bytesPerFrame*60/(1024*1024),
			float64(2_130_444_288)/(bytesPerFrame*60))
	}
	if imagesPerFrame > imagesPerFrameBound {
		failed = true
		t.Errorf("image allocation throughput=%.3f images/frame over %d frames (bound=%.3f). "+
			"Steady-state playback should not allocate new ebiten images.",
			imagesPerFrame, measureFrames, imagesPerFrameBound)
	}

	if failed {
		dumpAllocBreakdown(t, totalAllocDelta, imageAllocsDelta, measureFrames)
	}
	t.Logf("playback alloc throughput: %.1f KB/frame over %d frames (bound=%.0f KB/frame); "+
		"%.3f images/frame (bound=%.3f); at 60 FPS this is %.2f MB/s",
		bytesPerFrame/1024, measureFrames, float64(bytesPerFrameBound)/1024,
		imagesPerFrame, imagesPerFrameBound,
		bytesPerFrame*60/(1024*1024))
}

// dumpAllocBreakdown logs the per-tag image-allocation breakdown so the test
// failure log directly identifies which cache is rebuilding per frame.
func dumpAllocBreakdown(t *testing.T, totalAlloc uint64, imageAllocs int64, frames int) {
	t.Helper()
	t.Logf("=== alloc breakdown for %d frames ===", frames)
	t.Logf("TotalAlloc delta: %d KB (%.1f KB/frame)", totalAlloc/1024, float64(totalAlloc)/float64(frames)/1024)
	t.Logf("Image allocs:    %d (%.3f/frame)", imageAllocs, float64(imageAllocs)/float64(frames))

	type tagCount struct {
		tag   string
		count int64
	}
	imagesByTagMu.Lock()
	pairs := make([]tagCount, 0, len(imagesByTag))
	for tag, n := range imagesByTag {
		pairs = append(pairs, tagCount{tag, n})
	}
	imagesByTagMu.Unlock()
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].count > pairs[j].count })

	t.Logf("--- images allocated by tag (top 20) ---")
	limit := len(pairs)
	if limit > 20 {
		limit = 20
	}
	for i := 0; i < limit; i++ {
		perFrame := float64(pairs[i].count) / float64(frames)
		t.Logf("  %-44s  %6d  (%.3f/frame)", pairs[i].tag, pairs[i].count, perFrame)
	}
	if len(pairs) == 0 {
		t.Logf("  (no tracked image allocations — leak is not in tracked allocators; "+
			"check untagged ebiten.NewImage callsites or non-image allocators)")
	}
	t.Logf("=== end alloc breakdown ===")
	_ = fmt.Sprintf // keep fmt import live for future use
}

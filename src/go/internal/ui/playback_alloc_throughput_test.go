//go:build test

package ui

import (
	"fmt"
	"runtime"
	"sort"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// WASM's single-threaded GC cannot keep up with sustained allocation, so throughput, not retention, runs it out
// of memory: Update AND Draw every frame, with no forced GC. Steady playback allocates no *ebiten.Image; a failure
// logs which tracked image tag allocates most.
func TestPlaybackAllocThroughput(t *testing.T) {
	withDefaultAudio(t)
	withDefaultStart(t, false)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	g.SetPlayFunc(func(instrument.ID, float64, ...float64) {})

	// Vanilla load, then play.
	if err := g.SetSubdivisions(8); err != nil {
		t.Fatalf("set subdiv: %v", err)
	}
	g.drum.SetBPM(120)
	buildSoakScene(t, g, 4 /* rows */, 8 /* nodes per row */)

	screen := ebiten.NewImage(1280, 720)

	// The first frames legitimately allocate caches.
	const warmupFrames = 120
	const measureFrames = 1500

	// Start playback BEFORE warm-up so steady-state is reached during warm-up.
	g.SetPlaying(true)
	advanceFrames(g, 5)
	for i := 0; i < warmupFrames; i++ {
		_ = g.Update()
		g.Draw(screen)
	}

	ResetImageMetrics()

	var pre runtime.MemStats
	runtime.ReadMemStats(&pre)

	// No GC forcing: raw allocation throughput.
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

	// About 5 MB/s at 60 FPS, which WASM's GC sustains.
	const bytesPerFrameBound = 80 * 1024

	// Steady state allocates no image; the slack is for an occasional resize.
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

// Names the cache rebuilding per frame.
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
		t.Logf("  (no tracked image allocations — leak is not in tracked allocators; " +
			"check untagged ebiten.NewImage callsites or non-image allocators)")
	}
	t.Logf("=== end alloc breakdown ===")
	_ = fmt.Sprintf // keep fmt import live for future use
}

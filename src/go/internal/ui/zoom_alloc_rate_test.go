//go:build test

// Regression test for the user-reported bug "zooming in/out during playback
// hurts audio a lot".
//
// Root cause: the grid pane's tile/grid caches and the edge cache key on the
// (effectively continuous) camera scale — gridCache on exact cam.Scale
// (grid_pane_draw.go), gridTile on stepPx = round(scale*Step) which still ticks
// every ~1-2 frames of a continuous zoom, and the edge cache likewise. So during
// a zoom gesture every frame is a 100% cache miss: each of gridTile, gridCache and
// edgeCache re-rasterizes and allocates a fresh GPU texture EVERY frame. On the
// single WASM thread the resulting per-frame texture uploads stall Draw long
// enough to starve the sequencer goroutine, so audio events fire late / choppy.
//
// This pins the upstream invariant deterministically (no WASM / WebAudio needed):
// a continuous zoom must NOT allocate textures per frame for these caches. The
// end-to-end browser proxy is src/js/webaudio_zoom_stress.browser.test.js, which
// measures the same thing as +image-allocs/frame.

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestZoomDoesNotAllocPerFrame drives a continuous camera zoom across many frames
// during playback and asserts the grid/edge image caches do not re-rasterize (and
// thus allocate) on every frame. Pre-fix, each of gridTile/gridCache/edgeCache
// allocates ~1 image per frame, so N zoom frames produce ~N allocations per tag.
// A correct zoom GPU-scales the existing cached textures during the gesture and
// only re-rasterizes when the scale settles, keeping per-tag growth near zero.
func TestZoomDoesNotAllocPerFrame(t *testing.T) {
	withDefaultAudio(t)
	withDefaultStart(t, false)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	g.SetPlayFunc(func(string, float64, ...float64) {})
	g.drum.SetBPM(120)
	buildSoakScene(t, g, 2, 4)

	scratch := ebiten.NewImage(1280, 720)
	g.SetPlaying(true)

	// Warm-up at a fixed scale so every cache reaches its steady-state
	// allocation high-water mark before we start zooming.
	g.cam.Scale = 1.0
	g.cam.Snap()
	for i := 1; i <= 32; i++ {
		setPlayStartForAbs(g, i)
		g.engine.Predictor.Ensure(i + 1)
		_ = g.Update()
		g.Draw(scratch)
	}

	// Snapshot per-tag allocation counts after warm-up.
	baselineByTag := make(map[string]int64)
	imagesByTagMu.Lock()
	for k, v := range imagesByTag {
		baselineByTag[k] = v
	}
	imagesByTagMu.Unlock()

	// Drive a continuous zoom: oscillate cam.Scale between 0.4 and 3.0, a fresh
	// distinct scale every frame — exactly what a pinch / scroll-wheel gesture
	// produces. The playhead keeps advancing so playback stays live.
	const frames = 200
	dir := 1.0
	for i := 33; i < 33+frames; i++ {
		if g.cam.Scale > 3.0 {
			dir = -1
		} else if g.cam.Scale < 0.4 {
			dir = 1
		}
		// ~2.5%/frame, mirroring the browser test's zoomAt(delta=5).
		g.cam.Scale *= 1.0 + dir*0.025
		g.cam.Snap()
		setPlayStartForAbs(g, i)
		g.engine.Predictor.Ensure(i + 1)
		_ = g.Update()
		g.Draw(scratch)
	}

	imagesByTagMu.Lock()
	defer imagesByTagMu.Unlock()

	// The caches that the bug re-rasterizes every zoom frame. A correct zoom keeps
	// each of these well under one alloc per frame; pre-fix they grow ~= frames.
	zoomChurnTags := []string{"gridTile", "gridCache", "edgeCache"}
	const maxRebuildsPerTag = 20 // generous: allows settle re-rasters, kills per-frame churn

	t.Logf("per-tag allocation deltas across %d continuous-zoom frames:", frames)
	for k, v := range imagesByTag {
		if d := v - baselineByTag[k]; d > 0 {
			t.Logf("  %s += %d", k, d)
		}
	}

	for _, tag := range zoomChurnTags {
		got := imagesByTag[tag] - baselineByTag[tag]
		if got > maxRebuildsPerTag {
			t.Errorf("%q allocated %d images across %d continuous-zoom frames "+
				"(max %d) — the cache is re-rasterizing on every scale change. "+
				"During an active zoom it must GPU-scale the existing texture and "+
				"only re-rasterize once the scale settles. See grid_pane_draw.go "+
				"(gridTile/gridCache) and the edge cache.",
				tag, got, frames, maxRebuildsPerTag)
		}
	}
}

// TestGridRebuildBlitsBoundedWhenZoomedOut pins the SECOND, distinct zoom bug:
// zooming OUT a lot makes a single grid-cache rebuild explode. The grid tiles a
// stepPx-sized tile across the padded cache, so a naive rebuild does ~area/stepPx²
// DrawImage blits. As the camera zooms out, stepPx shrinks and the blit count
// grows QUADRATICALLY — at minimum zoom it is tens of thousands of blits per
// frame, which blocks Draw on the single WASM thread long enough to starve the
// sequencer goroutine. The user reports audio that degrades worse the further out
// they zoom and stays bad after the gesture settles.
//
// This is NOT caught by the allocation test above (reuse keeps allocations at
// zero regardless of blit count). The fix is to tile a multi-cell block so the
// per-rebuild blit count stays bounded across the whole zoom range.
func TestGridRebuildBlitsBoundedWhenZoomedOut(t *testing.T) {
	withDefaultAudio(t)
	withDefaultStart(t, false)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	g.SetPlayFunc(func(string, float64, ...float64) {})
	buildSoakScene(t, g, 2, 4)

	scratch := ebiten.NewImage(1280, 720)

	// A single grid-cache rebuild must never blit more than this many tiles,
	// regardless of zoom level. A naive stepPx tiling blows past it the moment the
	// camera zooms out far; a multi-cell block tile keeps it bounded.
	const maxBlitsPerRebuild = 1500

	// Sweep from default zoom down to the minimum (0.1). At each level force a
	// fresh rebuild (the scale differs from the cached scale) and count the blits
	// that single rebuild performs.
	scales := []float64{1.0, 0.5, 0.3, 0.2, 0.15, 0.1}
	// Settle at a scale far from the first probe so the first measured draw rebuilds.
	g.cam.Scale = 2.0
	g.cam.Snap()
	g.Draw(scratch)

	worst := int64(0)
	worstScale := 0.0
	for _, s := range scales {
		g.cam.Scale = s
		g.cam.Snap()
		ResetImageMetrics()
		g.Draw(scratch) // exactly one rebuild at this scale
		blits := MetricGridTileBlits()
		t.Logf("scale=%.2f grid rebuild blits=%d", s, blits)
		if blits > worst {
			worst, worstScale = blits, s
		}
	}

	if worst > maxBlitsPerRebuild {
		t.Errorf("a single grid-cache rebuild blitted %d tiles at scale %.2f "+
			"(max %d). The grid tiles a stepPx-sized tile, so the blit count grows "+
			"~area/stepPx² — quadratically as the camera zooms out — blocking Draw "+
			"on the single WASM thread and starving audio. Tile a multi-cell block "+
			"so the blit count stays bounded across zoom levels. See "+
			"grid_pane_draw.go tiling loop and buildGridTile.",
			worst, worstScale, maxBlitsPerRebuild)
	}
}

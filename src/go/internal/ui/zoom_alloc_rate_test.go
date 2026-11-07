//go:build test

// The grid and edge caches key on an effectively continuous camera scale, so a naive zoom misses every frame and
// uploads fresh textures that stall Draw on the single WASM thread and starve the sequencer. A continuous zoom must
// not allocate textures per frame.

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// A correct zoom GPU-scales the cached textures during the gesture and re-rasterises only when the scale settles.
func TestZoomDoesNotAllocPerFrame(t *testing.T) {
	withDefaultAudio(t)
	withDefaultStart(t, false)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	g.SetPlayFunc(func(instrument.ID, float64, ...float64) {})
	g.drum.SetBPM(120)
	buildSoakScene(t, g, 2, 4)

	scratch := ebiten.NewImage(1280, 720)
	g.SetPlaying(true)

	// Every cache reaches its steady-state high-water mark first.
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

	// A distinct scale every frame, as a pinch or wheel gesture produces, with playback live.
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

// A rebuild tiling a stepPx-sized tile does ~area/stepPx² blits, quadratic as the camera zooms out, blocking Draw
// at low zoom. Reuse keeps allocations at zero whatever the blit count, so the allocation test cannot see it; a
// multi-cell block tile keeps it bounded.
func TestGridRebuildBlitsBoundedWhenZoomedOut(t *testing.T) {
	withDefaultAudio(t)
	withDefaultStart(t, false)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	g.SetPlayFunc(func(instrument.ID, float64, ...float64) {})
	buildSoakScene(t, g, 2, 4)

	scratch := ebiten.NewImage(1280, 720)

	// Whatever the zoom level.
	const maxBlitsPerRebuild = 1500

	// Each level differs from the cached scale, forcing a fresh rebuild.
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

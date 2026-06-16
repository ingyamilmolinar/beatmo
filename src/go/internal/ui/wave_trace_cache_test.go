//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
	"github.com/ingyamilmolinar/beatmo/internal/audio"

	"github.com/hajimehoshi/ebiten/v2"
)

// setupWaveGame drives the Wave tab with a synthetic analyzer state fed through
// the same override the audio-panel pixel tests use, and returns the game so
// the caller can swap the override to simulate a 30 Hz analyzer republish.
func setupWaveGame(t *testing.T) *Game {
	t.Helper()
	g := driveScene(t, "crop_eq_tab_wave")
	snap := snapshotWithSine()
	rowSnaps := make([]RowSnapshot, 0, len(g.drum.Rows))
	for _, r := range g.drum.Rows {
		if r != nil && r.Instrument != "" {
			rowSnaps = append(rowSnaps, RowSnapshot{ID: r.Instrument, Name: r.Name, Snap: snap})
		}
	}
	testAnalyzerStateOverride = SynthesizeAnalyzerState("main", "Master", snap, rowSnaps, nil, "", "", audio.SampleRate())
	t.Cleanup(func() { testAnalyzerStateOverride = nil })
	g.drum.eqPanelZone.SetActiveTab(TabWave)
	return g
}

func newWaveAnalyzerOverride(g *Game) *analyzer.State {
	snap := snapshotWithSine()
	rowSnaps := make([]RowSnapshot, 0, len(g.drum.Rows))
	for _, r := range g.drum.Rows {
		if r != nil && r.Instrument != "" {
			rowSnaps = append(rowSnaps, RowSnapshot{ID: r.Instrument, Name: r.Name, Snap: snap})
		}
	}
	return SynthesizeAnalyzerState("main", "Master", snap, rowSnaps, nil, "", "", audio.SampleRate())
}

// TestWaveTraceCacheHitsAndInvalidates mirrors the Chain-tab guard: an
// unchanged analyzer state must collapse the Wave draw to a single blit, and a
// new *analyzer.State pointer (what AnalyzerState() returns when data refreshes
// at ~30 Hz) must rebuild. Identity, not a timestamp — the WASM analyzer path
// stamps none.
func TestWaveTraceCacheHitsAndInvalidates(t *testing.T) {
	defer SetWaveTraceCacheForTest(true)()
	g := setupWaveGame(t)
	screen := ebiten.NewImage(1280, 720)

	draw := func() int64 {
		ResetImageMetrics()
		g.Draw(screen)
		return MetricDrawCallsTotal()
	}

	cold := draw()
	hit := draw()
	if hit*4 > cold {
		t.Fatalf("expected wave cache hit far cheaper than cold render: cold=%d hit=%d", cold, hit)
	}

	testAnalyzerStateOverride = newWaveAnalyzerOverride(g)
	miss := draw()
	if miss*4 < cold {
		t.Fatalf("expected a new analyzer state pointer to rebuild the wave (~cold cost): cold=%d miss=%d", cold, miss)
	}

	again := draw()
	if again*4 > cold {
		t.Fatalf("expected wave cache hit after re-render: cold=%d again=%d", cold, again)
	}
	t.Logf("cold=%d hit=%d miss=%d again=%d", cold, hit, miss, again)
}

// TestWaveTraceCacheRenderMatchesDirect proves equivalence-by-construction: the
// filled-rect shape multiset is identical whether the waveform renders directly
// to the screen (cache off) or into the offscreen cache (cache on, cold). The
// blit translates the cached pixels back to the content rect, so identical
// shapes ⇒ identical on-screen pixels.
func TestWaveTraceCacheRenderMatchesDirect(t *testing.T) {
	g := setupWaveGame(t)
	screen := ebiten.NewImage(1280, 720)

	restoreOff := SetWaveTraceCacheForTest(false)
	g.Draw(screen) // warm layout
	direct := shapeHistogram(collectFilledRects(t, func() { g.Draw(screen) }))
	restoreOff()

	restoreOn := SetWaveTraceCacheForTest(true)
	defer restoreOn()
	cached := shapeHistogram(collectFilledRects(t, func() { g.Draw(screen) }))

	if len(direct) == 0 {
		t.Fatal("no filled rects captured in direct wave render")
	}
	if len(direct) != len(cached) {
		t.Fatalf("distinct shape count differs: direct=%d cached=%d", len(direct), len(cached))
	}
	for s, n := range direct {
		if cached[s] != n {
			t.Fatalf("shape %+v count differs: direct=%d cached=%d", s, n, cached[s])
		}
	}
	t.Logf("wave filled-rect shape multiset identical: %d distinct shapes", len(direct))
}

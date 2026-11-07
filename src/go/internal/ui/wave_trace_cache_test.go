//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/channel"

	"github.com/hajimehoshi/ebiten/v2"
)

// The caller swaps the override to simulate a ~30 Hz republish.
func setupWaveGame(t *testing.T) *Game {
	t.Helper()
	g := driveScene(t, "crop_eq_tab_wave")
	snap := snapshotWithSine()
	rowSnaps := make([]RowSnapshot, 0, len(g.drum.Rows))
	for _, r := range g.drum.Rows {
		if r != nil && r.Instrument != "" {
			rowSnaps = append(rowSnaps, RowSnapshot{ID: channel.Of(r.Instrument), Name: r.Name, Snap: snap})
		}
	}
	testAnalyzerStateOverride = SynthesizeAnalyzerState(channel.Master, "Master", snap, rowSnaps, nil, channel.ID{}, "", audio.SampleRate())
	t.Cleanup(func() { testAnalyzerStateOverride = nil })
	g.drum.eqPanelZone.SetActiveTab(TabWave)
	return g
}

func newWaveAnalyzerOverride(g *Game) *analyzer.State {
	snap := snapshotWithSine()
	rowSnaps := make([]RowSnapshot, 0, len(g.drum.Rows))
	for _, r := range g.drum.Rows {
		if r != nil && r.Instrument != "" {
			rowSnaps = append(rowSnaps, RowSnapshot{ID: channel.Of(r.Instrument), Name: r.Name, Snap: snap})
		}
	}
	return SynthesizeAnalyzerState(channel.Master, "Master", snap, rowSnaps, nil, channel.ID{}, "", audio.SampleRate())
}

// An unchanged state is one blit; a new *analyzer.State rebuilds. Identity, not a timestamp: the WASM path stamps none.
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

// The same filled-rect shape multiset direct or into the cold cache; the blit translates it back.
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

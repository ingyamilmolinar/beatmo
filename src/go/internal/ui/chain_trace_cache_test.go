//go:build test

package ui

import (
	"image/color"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/scope"

	"github.com/hajimehoshi/ebiten/v2"
)

// Returns the live override so the caller can simulate a ~30 Hz republish.
func setupChainScopeGame(t *testing.T) (*Game, *scope.State) {
	t.Helper()
	g := driveScene(t, "crop_chain_default")
	snap := snapshotWithSine()
	st := SynthesizeScopeState(channel.Master, scope.StageSynth, scope.StageEQ,
		ScopeStageSnapshots{Synth: snap, PreEQ: snap, PostEQ: snap, Master: snap})
	st.Timestamp = 1
	testScopeStateOverride = st
	t.Cleanup(func() { testScopeStateOverride = nil })
	g.drum.eqPanelZone.SetActiveTab(TabScope)
	return g, st
}

// An unchanged state is one blit, not thousands of per-column blits, which hands the sequencer a cheap frame;
// a republished state rebuilds.
func TestChainTraceCacheHitsAndInvalidates(t *testing.T) {
	defer SetChainTraceCacheForTest(true)()
	g, _ := setupChainScopeGame(t)
	screen := ebiten.NewImage(1280, 720)

	draw := func() int64 {
		ResetImageMetrics()
		g.Draw(screen)
		return MetricDrawCallsTotal()
	}

	cold := draw() // first draw: cache miss, full trace render
	hit := draw()  // same state pointer: cache hit, blit only
	if hit*4 > cold {
		t.Fatalf("expected cache hit to be far cheaper than cold render: cold=%d hit=%d", cold, hit)
	}

	// Keyed on identity, not Timestamp: the WASM path leaves Timestamp 0.
	snap := snapshotWithSine()
	testScopeStateOverride = SynthesizeScopeState(channel.Master, scope.StageSynth, scope.StageEQ,
		ScopeStageSnapshots{Synth: snap, PreEQ: snap, PostEQ: snap, Master: snap})
	miss := draw()
	if miss*4 < cold {
		t.Fatalf("expected a new scope state pointer to rebuild the trace (~cold cost): cold=%d miss=%d", cold, miss)
	}

	again := draw() // same (new) pointer still current → hit again
	if again*4 > cold {
		t.Fatalf("expected cache hit after re-render: cold=%d again=%d", cold, again)
	}
	t.Logf("cold=%d hit=%d miss=%d again=%d", cold, hit, miss, again)
}

// Size and colour only: the cache draws at a (0,0) origin, so equal shape multisets mean the same primitives.
type rectShape struct {
	w, h int
	c    color.RGBA
}

func shapeHistogram(rects []drawnRect) map[rectShape]int {
	h := map[rectShape]int{}
	for _, r := range rects {
		h[rectShape{r.Rect.Dx(), r.Rect.Dy(), r.Color}]++
	}
	return h
}

func toRGBA(c color.Color) color.RGBA { return color.RGBAModel.Convert(c).(color.RGBA) }

// One shared renderer for both paths, so the shape multisets match whether drawn direct or into the cold cache,
// and the blit translates them back.
func TestChainTraceCacheRenderMatchesDirect(t *testing.T) {
	g, _ := setupChainScopeGame(t)
	screen := ebiten.NewImage(1280, 720)

	restoreOff := SetChainTraceCacheForTest(false)
	g.Draw(screen) // warm layout
	direct := shapeHistogram(collectFilledRects(t, func() { g.Draw(screen) }))
	restoreOff()

	// First cache-on draw is a cold miss → full render into the offscreen cache.
	restoreOn := SetChainTraceCacheForTest(true)
	defer restoreOn()
	cached := shapeHistogram(collectFilledRects(t, func() { g.Draw(screen) }))

	if len(direct) == 0 {
		t.Fatal("no filled rects captured in direct render")
	}
	// The extra blit is a DrawImage, which collectFilledRects does not record.
	if len(direct) != len(cached) {
		t.Fatalf("distinct shape count differs: direct=%d cached=%d", len(direct), len(cached))
	}
	for s, n := range direct {
		if cached[s] != n {
			t.Fatalf("shape %+v count differs: direct=%d cached=%d", s, n, cached[s])
		}
	}
	t.Logf("filled-rect shape multiset identical: %d distinct shapes", len(direct))
}

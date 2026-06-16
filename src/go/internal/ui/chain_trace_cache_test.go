//go:build test

package ui

import (
	"image/color"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/scope"

	"github.com/hajimehoshi/ebiten/v2"
)

// setupChainScopeGame drives the Chain tab with a synthetic scope state fed
// through the same override path the audio-panel pixel tests use, and returns
// the game plus the live *scope.State override so the caller can mutate its
// Timestamp to simulate a 30 Hz scope republish.
func setupChainScopeGame(t *testing.T) (*Game, *scope.State) {
	t.Helper()
	g := driveScene(t, "crop_chain_default")
	snap := snapshotWithSine()
	st := SynthesizeScopeState("main", scope.StageSynth, scope.StageEQ,
		ScopeStageSnapshots{Synth: snap, PreEQ: snap, PostEQ: snap, Sends: snap, Master: snap})
	st.Timestamp = 1
	testScopeStateOverride = st
	t.Cleanup(func() { testScopeStateOverride = nil })
	g.drum.eqPanelZone.SetActiveTab(TabScope)
	return g, st
}

// TestChainTraceCacheHitsAndInvalidates proves the cache removes the trace
// render from frames where the scope state is unchanged (a cache hit is a
// single blit, not ~thousands of per-column blits) and rebuilds when the
// scope service republishes a new state (Timestamp changes). This is the
// mechanism that hands the sequencer goroutine a cheap frame between renders
// and stops the audio stuttering under the Chain tab.
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

	// A new *scope.State (what ScopeState() returns when the data refreshes —
	// every ~30 Hz tick on desktop / stateCacheTTL on WASM) must rebuild the
	// trace. Identity, not Timestamp: the WASM path leaves Timestamp 0.
	snap := snapshotWithSine()
	testScopeStateOverride = SynthesizeScopeState("main", scope.StageSynth, scope.StageEQ,
		ScopeStageSnapshots{Synth: snap, PreEQ: snap, PostEQ: snap, Sends: snap, Master: snap})
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

// rectShape is a position-independent fingerprint of a filled rect: its size
// and color. The cache renders the trace at a (0,0) origin instead of at the
// content rect, so the on-screen positions differ by a constant offset that
// the blit reproduces — but the SHAPES drawn are identical. Comparing the
// multiset of shapes therefore proves the cached and direct renders emit the
// exact same primitives (pixel-equivalent once translated).
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

// TestChainTraceCacheRenderMatchesDirect proves equivalence-by-construction:
// on the same game/frame, the multiset of filled-rect shapes is identical
// whether the trace renders directly to the screen (cache off) or into the
// offscreen cache (cache on, cold). drawChainTraces is the one shared renderer
// in both paths, differing only in the destination and the rect origin, so any
// shape difference would mean the cache draws something different. The blit
// translates the cached pixels back to the content rect, so identical shapes ⇒
// identical on-screen pixels.
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
	// The cache-on draw additionally issues exactly one DrawImage blit, which
	// collectFilledRects does not record (it only records filled drawRect), so
	// the filled-rect shape multisets must be exactly equal.
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

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// During follow-scroll the row content only translates, so the cache keeps a strip wider than the window and
// recomposites only past its pad: on WASM's one thread a per-scroll copy steals time from the sequencer. Runs
// with the production TrackBeat refresh, since the test default force-dirties every frame.
func TestRowsLayerDoesNotRecomposeEveryScrollDuringPlayback(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultAudio(t)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(900, 600)

	// Minimal looping circuit so the playhead advances and the window scrolls.
	n0 := g.tryAddNode(0, 0, 0)
	g.start = n0
	g.graph.StartNodeID = n0.ID
	n1 := g.tryAddNode(1, 0, 0)
	g.addEdge(n0, n1)
	g.addEdge(n1, n0)
	g.updateBeatInfos()

	g.SetPlayFunc(func(instrument.ID, float64, ...float64) {})
	g.drum.SetFollow(true)
	defer SetTrackBeatForceRefreshForTest(false)() // measure PRODUCTION cadence
	pressPlay(t, g.drum)

	scr := ebiten.NewImage(900, 600)

	// Past the 70% recenter point, so this measures steady-state scrolling, not the warm-up.
	steps := 256
	prevGen := g.drum.rowsLayerGen
	prevOffset := g.drum.Offset
	recomposites := 0
	scrollEvents := 0
	contentChanges := 0
	var lastSig uint64
	for i := 1; i <= steps; i++ {
		setPlayStartForAbs(g, i)
		_ = g.Update()
		g.Draw(scr)
		if g.drum.rowsLayerGen != prevGen {
			recomposites += int(g.drum.rowsLayerGen - prevGen)
			prevGen = g.drum.rowsLayerGen
		}
		if g.drum.Offset != prevOffset {
			scrollEvents++
			prevOffset = g.drum.Offset
		}
		if len(g.rowRenderSig) > 0 {
			if i > 1 && g.rowRenderSig[0] != lastSig {
				contentChanges++
			}
			lastSig = g.rowRenderSig[0]
		}
	}

	t.Logf("steps=%d recomposites=%d scrollEvents=%d contentChanges=%d", steps, recomposites, scrollEvents, contentChanges)

	// Sanity: the window actually scrolled (otherwise the test proves nothing).
	if scrollEvents < 20 {
		t.Fatalf("expected steady-state scrolling (scrollEvents>=20), got %d — test circuit/length is not exercising follow-scroll", scrollEvents)
	}
	// Sanity: the content did NOT change — so every recompose is pure waste.
	if contentChanges != 0 {
		t.Fatalf("expected stable row content during steady playback, got %d content changes", contentChanges)
	}

	// Recomposites scale with scroll distance over the pad, not with scroll events; require a 4× reduction.
	maxRecomposites := scrollEvents/4 + 2
	if recomposites > maxRecomposites {
		t.Fatalf("rows layer recomposited %d times over %d scroll events during steady playback "+
			"(content never changed) — expected <= %d. The per-scroll full-layer copy starves the "+
			"sequencer goroutine on WASM. Use a wider windowed cache + sub-rect blit.",
			recomposites, scrollEvents, maxRecomposites)
	}
}

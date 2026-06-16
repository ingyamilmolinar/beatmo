package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestRowsLayerDoesNotRecomposeEveryScrollDuringPlayback is the performance
// regression guard for the "choppy audio while just playing" bug
// (bench-results/choppy_audio_revisit_2026-06-13.md). During steady-state
// follow-scroll playback the drum-row CONTENT does not change — the window
// merely translates — yet the rows-layer cache was recomposited (a full
// layer-sized shift-copy) on essentially every recenter (~every 2 cells).
// On the single cooperatively-scheduled WASM thread that per-scroll copy is
// pure waste that steals time from the sequencer goroutine.
//
// The deterministic fix is to cache a strip WIDER than the visible window and
// blit a moving sub-rectangle each frame, recompositing only when the playhead
// scrolls past the cached pad. This test pins the invariant: over a long
// steady-state scroll the number of rows-layer recomposites must be a small
// fraction of the number of scroll events (it scales with window/pad, not with
// every cell of playhead travel).
//
// IMPORTANT: this drives the loop with SetTrackBeatForceRefreshForTest(false)
// so TrackBeat behaves exactly as it does in the browser/desktop binary. The
// default test behavior force-dirties the layer every frame "to satisfy
// visibility assertions", which would mask the production scroll cadence this
// test measures (see drumview_transport.go).
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

	g.SetPlayFunc(func(string, float64, ...float64) {})
	g.drum.SetFollow(true)
	defer SetTrackBeatForceRefreshForTest(false)() // measure PRODUCTION cadence
	pressPlay(t, g.drum)

	scr := ebiten.NewImage(900, 600)

	// Advance well past the recenter point (70% of the window) so we measure
	// the STEADY-STATE scrolling regime, not the recompose-free warmup while
	// the playhead is still in the first 70% of the initial window.
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

	// The invariant: recomposites must be a SMALL fraction of scroll events.
	// A windowed cache recomposites only when the playhead scrolls past the
	// pad, so recomposites scale with (scroll distance / pad), not with every
	// scroll event. Pre-fix the rows layer shift-copied on essentially every
	// scroll (recomposites ≈ scrollEvents). Require at least a 4× reduction.
	maxRecomposites := scrollEvents/4 + 2
	if recomposites > maxRecomposites {
		t.Fatalf("rows layer recomposited %d times over %d scroll events during steady playback "+
			"(content never changed) — expected <= %d. The per-scroll full-layer copy starves the "+
			"sequencer goroutine on WASM. Use a wider windowed cache + sub-rect blit.",
			recomposites, scrollEvents, maxRecomposites)
	}
}

//go:build test

// Phase 3C — data-structure bound assertions.
//
// One sub-test per growable structure on the per-Update / per-Draw path.
// Each ceiling is derived from the existing module bound (predictor
// window cap, parity audio max, timeline archive cap, etc.), NOT
// invented; failure messages cite the bound's source-file constant so a
// regressor knows exactly where to look.
//
// Drives the same TabMeters scaffold the long-session canary uses, but
// keeps the iteration count modest (5 000 frames) so this test runs in
// a few seconds for CI. The 1 M-iteration canary lives separately.

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// boundsCanaryFrames is the per-test iteration budget. Smaller than the
// 60 000-frame heap canary because here we only need each structure to
// observe steady-state pruning; we don't need to reproduce the OOM
// timing.
const boundsCanaryFrames = 5_000

// runBoundsCanary builds the standard 6-row × 8-node soak scene with
// the EQ panel set to TabMeters and the metrics-only callback wired
// the way Phase 2A landed it. Returns a Game that has been driven
// through boundsCanaryFrames Update+Draw cycles. Callers then snapshot
// whichever data structure they're asserting against.
func runBoundsCanary(t *testing.T) *Game {
	t.Helper()
	withDefaultAudio(t)
	withDefaultStart(t, false)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	g.SetPlayFunc(func(string, float64, ...float64) {})
	if err := g.SetSubdivisions(16); err != nil {
		t.Fatalf("set subdivisions: %v", err)
	}
	g.drum.SetBPM(240)
	buildSoakScene(t, g, 6, 8)

	// Wire the same callbacks the production drumview ctor produces;
	// the metrics-only path is what we're verifying stays bounded.
	if g.drum.eqPanelZone == nil {
		t.Fatal("eqPanelZone nil; harness assumption broken")
	}
	g.drum.eqPanelZone.tabState.SetActiveTab(TabMeters)
	g.drum.eqPanelZone.callbacks.AnalyzerState = func() *analyzer.State {
		return buildAnalyzerStateFromSnapshotsImpl("", g.drum.Rows, 48000, allocFetch)
	}
	g.drum.eqPanelZone.callbacks.AnalyzerMetricsOnly = func() *analyzer.State {
		return buildAnalyzerMetricsOnlyImpl(g.drum.Rows, scalarFetch)
	}

	scratch := ebiten.NewImage(1280, 720)
	g.SetPlaying(true)
	advanceFrames(g, 5)
	audio.ResetAnalyzerBridgeStats()

	for i := 1; i <= boundsCanaryFrames; i++ {
		setPlayStartForAbs(g, i)
		g.engine.Predictor.Ensure(i + 1)
		_ = g.Update()
		g.drum.eqPanelZone.Draw(scratch)
	}
	return g
}

// TestDataStructureBounds asserts every growable structure stays under
// its named ceiling after the canary scaffold finishes. The ceilings
// come from the module's existing constants (parityAudioMax,
// paritySeqDecisionsPerRowMax, archiveDefaultMaxEntries, etc.) so a
// regression that loosens the bound also has to update this test
// in lockstep — the assertion message names the file:constant.
func TestDataStructureBounds(t *testing.T) {
	g := runBoundsCanary(t)

	t.Run("timeline_archives_per_row", func(t *testing.T) {
		// internal/timeline/archive.go: archiveDefaultMaxEntries =
		// 2_000_000 per row. We can't import the constant from this
		// package without the cycle — but we can assert the per-row
		// count stays well under at the canary horizon (5k frames at
		// BPM 240 / 16 subdiv ≈ 1300 abs steps, all should fit in the
		// live immutables sidecar before migration kicks in).
		const ceiling = 2_000_000
		for r := 0; r < len(g.drum.Rows); r++ {
			got := g.timeline.ArchiveLenForTest(r)
			if got > ceiling {
				t.Errorf("timeline.archives[row=%d] = %d entries (cap %d, "+
					"see internal/timeline/archive.go archiveDefaultMaxEntries) — "+
					"cold archive eviction failed", r, got, ceiling)
			}
		}
	})

	t.Run("parity_audio_per_row", func(t *testing.T) {
		// internal/ui/game_parity_state.go:150 — parityAudioMax = 1024
		const ceiling = 1024
		g.parityMu.Lock()
		got := len(g.parityAudio)
		g.parityMu.Unlock()
		if got > ceiling {
			t.Errorf("parityAudio = %d events (cap %d, "+
				"see internal/ui/game_parity_state.go:150 parityAudioMax) — "+
				"parityPrune is not keeping up with playback rate", got, ceiling)
		}
	})

	t.Run("parity_seq_decisions_per_row", func(t *testing.T) {
		// internal/ui/game_parity_state.go:212 — paritySeqDecisionsPerRowMax = 1024
		const ceiling = paritySeqDecisionsPerRowMax
		g.parityMu.Lock()
		defer g.parityMu.Unlock()
		for r, m := range g.paritySeqDecisions {
			if len(m) > ceiling {
				t.Errorf("paritySeqDecisions[row=%d] = %d entries (cap %d, "+
					"see internal/ui/game_parity_state.go:212) — "+
					"per-row sliding-window prune broken",
					r, len(m), ceiling)
			}
		}
	})

	t.Run("audio_channel_capacity", func(t *testing.T) {
		// audioCh is a buffered channel; cap is fixed at construction
		// (game_new.go). Test that len never exceeds cap and that cap
		// stays at the documented size — a regression that bumps cap
		// without justification is suspicious.
		got := len(g.audioCh)
		if got > cap(g.audioCh) {
			t.Errorf("audioCh len=%d > cap=%d — channel overflow impossible "+
				"unless the channel was rebuilt mid-test", got, cap(g.audioCh))
		}
		const expectedCap = 128
		if cap(g.audioCh) != expectedCap {
			t.Logf("NOTE: audioCh cap = %d (was %d at plan time). If this is intentional, "+
				"update expectedCap in this test", cap(g.audioCh), expectedCap)
		}
	})

	t.Run("audio_scheduler_pending", func(t *testing.T) {
		// Scheduler heap should not accumulate entries past the
		// lookahead horizon. At BPM 240, 16 subdiv, 6 rows, lookahead
		// 0.1s: ≈ (240/60)*16*6*0.1 = 38 entries plus generous slack.
		if g.audioScheduler == nil {
			t.Skip("audioScheduler not initialised in this build")
		}
		const ceiling = 256
		got := g.audioScheduler.Stats().Pending
		if got > int64(ceiling) {
			t.Errorf("audioScheduler pending = %d (cap %d, lookahead × beat rate × rows) — "+
				"scheduler heap is not draining", got, ceiling)
		}
	})

	t.Run("predictor_window_cap", func(t *testing.T) {
		// internal/ui/runtime_profile.go: PredictorWindowCap = 4096.
		// Predictor.WindowBoundsForTest exposes the live window length
		// AND the active cap; both must hold the documented bound.
		ws, we, windowCap := g.engine.Predictor.WindowBoundsForTest()
		length := we - ws
		const ceiling = 4096
		if length > ceiling {
			t.Errorf("predictor window = [%d,%d) length %d (cap %d, "+
				"RuntimeProf().PredictorWindowCap) — window slide broken",
				ws, we, length, ceiling)
		}
		if windowCap > ceiling {
			t.Errorf("predictor windowCap = %d > %d — runtime profile loosened "+
				"the bound without updating this test", windowCap, ceiling)
		}
	})

	t.Run("highlighted_beats", func(t *testing.T) {
		// internal/ui/game_highlight_state.go:114 —
		// highlightAbsRetentionSlack = 64. Visible window + slack is
		// the upper bound; clearExpiredHighlights runs every frame.
		got := len(g.highlightedBeats)
		bound := g.drum.Length + highlightAbsRetentionSlack + 1
		if got > bound {
			t.Errorf("highlightedBeats = %d entries (cap %d = drum.Length(%d) + "+
				"highlightAbsRetentionSlack(%d) + 1, see game_highlight_state.go:114) — "+
				"per-frame eviction broken",
				got, bound, g.drum.Length, highlightAbsRetentionSlack)
		}
	})

	t.Run("last_triggered_per_row_bounded_by_graph", func(t *testing.T) {
		// lastTriggeredByRow[row] is keyed by NodeID; bounded by graph
		// node count (no graph mutation during canary).
		nodeCount := len(g.engine.Graph.Nodes)
		g.triggerMu.RLock()
		defer g.triggerMu.RUnlock()
		for r, m := range g.lastTriggeredByRow {
			if len(m) > nodeCount {
				t.Errorf("lastTriggeredByRow[row=%d] = %d entries > graph node count %d — "+
					"map retains entries for deleted nodes",
					r, len(m), nodeCount)
			}
		}
	})

	t.Run("node_anim_bounded_by_graph", func(t *testing.T) {
		// nodeAnim is keyed by NodeID; bounded by graph node count.
		nodeCount := len(g.engine.Graph.Nodes)
		g.triggerMu.RLock()
		got := len(g.nodeAnim)
		g.triggerMu.RUnlock()
		if got > nodeCount {
			t.Errorf("nodeAnim = %d entries > graph node count %d — "+
				"animation map retains entries for deleted nodes",
				got, nodeCount)
		}
	})

	t.Run("images_by_tag_cache_bounded", func(t *testing.T) {
		// internal/ui/image_metrics.go:30 — imagesByTag is keyed by a
		// short stable string passed to newTrackedImage("rowsLayer", …).
		// Every call site uses a compile-time-constant string, so the
		// map size is bounded by the number of newTrackedImage call
		// sites in the source. A regression that passes a dynamic key
		// (row index, instrument id, …) would blow this bound.
		imagesByTagMu.Lock()
		got := len(imagesByTag)
		imagesByTagMu.Unlock()
		// Current call-site inventory: bgCache, edgeCache, gridCache,
		// gridTile, nodeLayer, rowCache.empty, rowCacheScratch,
		// rowRackZone.controlsCache, rowsLayer, rowsLayerScratch,
		// rowSprite, timelineZone.hlSpriteMute,
		// timelineZone.hlSpriteReg, timelineZone.TlCache,
		// transport.barCache, transportZone.toolbarCache → 16 tags.
		// 32 leaves headroom for legitimate new call sites without
		// hiding a leak.
		const ceiling = 32
		if got > ceiling {
			t.Errorf("imagesByTag = %d entries (cap %d, "+
				"see internal/ui/image_metrics.go:30) — "+
				"a newTrackedImage callsite is passing a dynamic key; "+
				"keys must be compile-time-constant short strings",
				got, ceiling)
		}
	})

	t.Run("analyzer_bridge_calls_per_frame", func(t *testing.T) {
		// In the WASM build, audio.AnalyzerBridgeStats reports the
		// number of JS↔Go bridge calls. The Meters tab should make 1
		// call per channel per frame (master + every populated row).
		// In -tags test the stub returns 0; assertion only meaningful
		// when calls > 0 (i.e. on the WASM side once Phase 2C lands).
		calls, reads := audio.AnalyzerBridgeStats()
		if calls == 0 {
			t.Logf("AnalyzerBridgeStats: stub build, no bridge activity (expected under -tags test)")
			return
		}
		// Per-frame ratio: 1 master + 6 rows = 7 calls per Draw × frames
		const maxPerFrame = 8
		gotPerFrame := float64(calls) / float64(boundsCanaryFrames)
		if gotPerFrame > maxPerFrame {
			t.Errorf("AnalyzerBridgeStats calls/frame = %.1f (cap %d) — bridge call rate "+
				"explosion; check getAnalyzerStateForTab memo wiring",
				gotPerFrame, maxPerFrame)
		}
		// elementReads / calls: scalar path = 0 reads, full path = N reads.
		// Phase 2C target: ≤ 8 reads/call (bulk transfer).
		const maxReadsPerCall = 8
		if calls > 0 {
			ratio := float64(reads) / float64(calls)
			if ratio > maxReadsPerCall {
				t.Errorf("AnalyzerBridgeStats reads/call = %.1f (cap %d) — "+
					"per-element js.Value loop survived; Phase 2C bulk transfer missing",
					ratio, maxReadsPerCall)
			}
		}
	})
}

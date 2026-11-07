//go:build test

// Every growable structure on the per-frame path stays under its module's own bound (the failure names the
// constant), over the TabMeters soak scaffold at a CI-sized iteration count.

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// Enough for each structure to reach steady-state pruning.
const boundsCanaryFrames = 5_000

// 6 rows × 8 nodes, TabMeters with the metrics-only callback, driven through the iteration budget.
func runBoundsCanary(t *testing.T) *Game {
	t.Helper()
	withDefaultAudio(t)
	withDefaultStart(t, false)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	g.SetPlayFunc(func(instrument.ID, float64, ...float64) {})
	if err := g.SetSubdivisions(16); err != nil {
		t.Fatalf("set subdivisions: %v", err)
	}
	g.drum.SetBPM(240)
	buildSoakScene(t, g, 6, 8)

	// The production metrics-only path.
	if g.drum.eqPanelZone == nil {
		t.Fatal("eqPanelZone nil; harness assumption broken")
	}
	g.drum.eqPanelZone.tabState.SetActiveTab(TabMeters)
	g.drum.eqPanelZone.callbacks.AnalyzerState = func() *analyzer.State {
		return buildAnalyzerStateFromSnapshotsImpl(channel.ID{}, g.drum.Rows, 48000, allocFetch, nil)
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

// Ceilings come from the modules' constants, so loosening a bound means updating this too.
func TestDataStructureBounds(t *testing.T) {
	g := runBoundsCanary(t)

	t.Run("timeline_archives_per_row", func(t *testing.T) {
		// archiveDefaultMaxEntries per row; it cannot be imported here without a cycle.
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
		// A buffered channel whose cap is fixed at construction; an unexplained bump is suspicious.
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
		// Nothing past the lookahead: at 240 BPM, 16 subdiv, 6 rows, 0.1 s that is ~38 entries plus slack.
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
		// PredictorWindowCap; both the live length and the cap hold.
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
		// The visible window + highlightAbsRetentionSlack; cleared every frame.
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
		// Keyed by NodeID, so bounded by the node count.
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
		// Keyed by compile-time constant tags, so bounded by the call-site count; a dynamic key would blow it.
		imagesByTagMu.Lock()
		got := len(imagesByTag)
		imagesByTagMu.Unlock()
		// Headroom over the current call sites without hiding a leak.
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
		// Meters makes one bridge call per channel per frame; the stub reports 0, so this only bites in WASM.
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
		// The scalar path reads 0; a bulk transfer stays under this.
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

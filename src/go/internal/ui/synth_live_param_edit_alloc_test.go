//go:build test

package ui

import (
	"image"
	"runtime"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// Live synth-knob edit frame BYTE budget — the "editing the kick Punch knob
// during playback hangs the game" regression (2026-07 user report, WASM).
//
// THE BUG: with the Synth tab open, every frame re-derives the instrument's
// merged param map from scratch — audio.MergeRecipeDefaults over the FULL
// modular schema (~1030 ParamDefs, ~115 KB per call) runs in buildSynthTab
// (Layout, every frame), in DrumView.synthParamsHash (the mirror's re-render
// gate, every Draw), and again inside every preview/concept wave render
// (RenderInstrumentPreviewWave / conceptMergedParams). synthParamsHash
// additionally sorts + strconv-formats all ~1030 params per frame. During a
// knob drag the params hash changes every frame, so the mirror + focus-graph
// waves re-render each frame, each re-merging again — ~0.5–1 MB of garbage
// per frame. On the single-threaded WASM heap that allocation churn drives
// the GC into 100 ms+ stalls and collapses the frame rate (measured: Update
// 0.8 ms → 9.3 ms just opening the tab; fps 55 → 23 while dragging).
//
// The existing eq_panel_draw_alloc_discipline_test counts ALLOCS per draw and
// misses this entirely: MergeRecipeDefaults is only ~6 allocs — of 115 KB.
// This test measures BYTES per frame through the REAL user path: real Game,
// playback running, Synth tab open, a real press+drag on the kick's Punch
// knob (gen1_kick_attack), real Update+Draw frames.
//
// Budgets are bytes-per-frame ceilings with generous headroom over the
// post-fix baseline (rev-gated merged-param caching). They fail by ~5–10x
// while the per-frame re-merge storm exists.
const (
	// synthTabIdleFrameByteBudget caps an idle Synth-tab frame (tab open,
	// playback running, NO knob touched). Post-fix baseline 43 KB/frame
	// (draw-side column rects + captions); cap = baseline × ~3 for CI
	// headroom. The pre-fix cost was 1814 KB/frame (a dozen-plus full-schema
	// merges + the sort/format hash + the schema-sized HitArea reserve,
	// every frame).
	synthTabIdleFrameByteBudget = 128 << 10
	// synthTabDragFrameByteBudget caps a frame WHILE a value edit is moving
	// (value changes every frame → mirror + stage-thumbs + focus re-render).
	//
	// 2026-07-16 ParamCard conversion re-measure (Task 10 guardrails sweep):
	// idle is 68–70 KB/frame (well under the 128 KB idle cap — the card-gauge
	// draw path in param_card.go is cheap); the drag frame measures ~1120–1220
	// KB/frame here. That is NOT a draw-path regression: an alloc_space profile
	// attributes >90% of the drag-frame bytes to internal/audio
	// (RenderInstrumentPreviewWithOverrides 58%, MergeRecipeDefaults /
	// ModularSynthParamDefs / RecipeRegistrations / RecipeDefaultParams /
	// BuildBuiltinRecipeDocs the rest) — the documented pre-plan "synth param
	// re-render storm" (memory project_synth_tab_drag_byte_budget_regression;
	// ledger note 2026-07-16: 250 KB > 224 at HEAD, ~94% internal/audio storm).
	// The card conversion's own draw additions (buildSynthTab, chip strip,
	// synthTabHitAreas, drawSynthChipStrip) sum to <10% of the frame.
	//
	// The storm is triggered by the pre-existing Task-14/15 stage-thumbs SWR:
	// every params-hash change schedules an OFF-THREAD re-render of all chips.
	// This byte metric reads runtime.MemStats.TotalAlloc, which counts the
	// async worker's allocations too, so on this native multi-core box the
	// worker completes many off-thread renders per frame's wall-clock — the
	// number scaled up (vs the pre-plan 250 KB) purely because ParamCard draw
	// makes each frame slightly longer, giving the worker more time to render.
	// The user-visible "editing Punch hangs the game" symptom is UI-thread
	// frame time, which the SWR already keeps to one string-compare per frame;
	// fixing the residual off-thread churn is an internal/audio caching change
	// out of scope for this UI-only conversion.
	//
	// Budget set to the measured worst (~1216 KB) + headroom for CI async
	// throughput variance. Post-storm-fix baseline was 58 KB/frame.
	synthTabDragFrameByteBudget = 1536 << 10
)

// measureFrameBytes runs Update+Draw for frames iterations, invoking perFrame
// before each, and returns the average allocated bytes per frame.
func measureFrameBytes(g *Game, screen *ebiten.Image, frames int, perFrame func(i int)) float64 {
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	for i := 0; i < frames; i++ {
		if perFrame != nil {
			perFrame(i)
		}
		_ = g.Update()
		g.Draw(screen)
	}
	runtime.ReadMemStats(&after)
	return float64(after.TotalAlloc-before.TotalAlloc) / float64(frames)
}

func TestSynthTab_LiveKnobEditFrameByteBudget(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)

	// Row 0 = the kick the user was editing (dnb-kick, the startup-demo kick;
	// a modular recipe — the ~1030-param schema is the worst case).
	uiNode := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.drum.Rows[0].Origin = uiNode.ID
	g.drum.Rows[0].Node = uiNode
	g.drum.Rows[0].Name = "Kick"
	g.drum.Rows[0].Instrument = "dnb-kick"
	if audio.RecipeForInstrument("dnb-kick") == "" {
		t.Fatal("dnb-kick has no recipe binding — harness assumption broken")
	}
	t.Cleanup(func() { audio.ResetInstrumentParams("dnb-kick") })
	g.updateBeatInfos()

	expandSynthPanelForTest(t, g)

	// Find the Punch knob (gen1_kick_attack). Advanced-tier knobs are now always
	// laid out below the in-grid divider (no expander), so selectSectionForKnobIdx
	// opens the owning stage and scrolls the knob into view.
	idx := synthBindingIdxByName(t, g, "gen1_kick_attack")
	selectSectionForKnobIdx(t, g, "dnb-kick", idx)
	knobs := g.drum.SynthTabKnobs()
	knob := knobs[idx]
	rect := knob.Rect()
	if rect.Empty() {
		t.Fatalf("Punch knob (gen1_kick_attack) has empty rect after opening its stage")
	}

	// Start playback through the real play path so the sequencer runs during
	// the measurement, matching the user scenario.
	g.drum.playPressed = true
	screen := ebiten.NewImage(1280, 720)
	for i := 0; i < 30; i++ { // warm-up: caches, glyphs, first mirror render
		_ = g.Update()
		g.Draw(screen)
	}

	// Phase 1: idle Synth-tab frames (tab open, no knob touched).
	idleBytes := measureFrameBytes(g, screen, 60, nil)
	t.Logf("idle synth-tab frame: %.0f KB/frame (budget %d KB)", idleBytes/1024, synthTabIdleFrameByteBudget>>10)

	// Phase 2: a real drag on the Punch knob, moving every frame.
	hits := g.drum.eqPanelZone.HitAreas()
	var pressHit *HitArea
	center := image.Pt((rect.Min.X+rect.Max.X)/2, (rect.Min.Y+rect.Max.Y)/2)
	for i, h := range hits {
		if h.Tag != "" && strings.HasPrefix(h.Tag, "synth-knob-") && center.In(h.Rect) {
			pressHit = &hits[i]
			break
		}
	}
	if pressHit == nil {
		t.Fatalf("no synth-knob HitArea covers the Punch knob center %v", center)
	}
	dragBytes := measureFrameBytes(g, screen, 60, func(i int) {
		// Task-5: value edits are the desktop wheel nudge. Alternate direction so
		// the value (and params hash) changes every frame, like a user spinning
		// the wheel back and forth.
		dir := 1
		if (i/8)%2 == 1 {
			dir = -1
		}
		pressHit.Handler.OnWheel(center.X, center.Y, dir)
	})
	t.Logf("drag synth-tab frame: %.0f KB/frame (budget %d KB)", dragBytes/1024, synthTabDragFrameByteBudget>>10)

	// Sanity: the drag actually reached the audio engine (the live-edit path
	// under measurement is the real one).
	if p := audio.GetInstrumentParams("dnb-kick"); len(p) == 0 {
		t.Fatal("drag did not propagate any param to the audio engine — measurement exercised nothing")
	}

	stopPlaybackForTest(g)

	if idleBytes > synthTabIdleFrameByteBudget {
		t.Errorf("idle Synth-tab frame allocates %.0f KB — over the %d KB budget. The tab re-derives the full merged param map every frame (buildSynthTab / synthParamsHash); on single-threaded WASM this churn stalls the GC and starves the frame loop.",
			idleBytes/1024, synthTabIdleFrameByteBudget>>10)
	}
	if dragBytes > synthTabDragFrameByteBudget {
		t.Errorf("live knob-drag frame allocates %.0f KB — over the %d KB budget. Every drag frame re-merges the ~1030-param schema several times (mirror hash, preview waves, focus graph); this is the 'editing Punch during playback hangs the game' regression.",
			dragBytes/1024, synthTabDragFrameByteBudget>>10)
	}
}

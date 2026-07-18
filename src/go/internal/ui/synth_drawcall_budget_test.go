//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestSynthTabSteadyStateDrawCallBudget pins the Synth-tab draw-call collapse
// from the TraceSprite adoption (bench-results/synth-cache-after.md). Before
// caching, a steady Synth-tab frame re-rasterized every stage-card keycap, the
// ~15-chip strip (each with a per-column waveform watermark), and the three
// per-column right-pane traces (full-note / up-close mirror / focus-graph
// concept curve) — hundreds of filled drawRect calls per frame, the draw cost
// that dominated the crackle bottleneck. With the five surfaces sprite-cached,
// a steady frame (unchanged params ⇒ every card/trace/chip sprite is a cache
// hit ⇒ its render callback never runs) issues only the uncached chrome rects
// plus a handful of cached blits.
//
// Mirrors TestEQTabSteadyStateDrawCallBudget. Budget = measured + headroom;
// tighten it (ratchet) if you cache more surfaces, and justify any INCREASE in
// a comment here — an increase means a surface regressed to per-frame raster.
func TestSynthTabSteadyStateDrawCallBudget(t *testing.T) {
	// Measured 2026-07-17 post-sprite-adoption on crop_synth_tab_modular
	// (OSC+FM+ENVELOPE+FILTER+POST cards + the right-pane traces): see the
	// t.Log below. The pre-cache count on the same scene was in the hundreds
	// (per-column trace + watermark rects). Ratchet down if you reduce further.
	// Measured 298 (near the EQ tab's 284 whole-screen chrome baseline — the
	// synth-specific per-column trace/watermark/card rasters are gone). 340 =
	// measured + ~14% headroom, tight enough that reverting even one surface
	// trips it (the focus-graph alone re-adds ~2 rects/px-column ⇒ ~600).
	const budget = 340

	g := driveScene(t, "crop_synth_tab_modular")
	screen := ebiten.NewImage(1280, 720)

	// Frame 1 warms every synth sprite (cache miss). Frame 2 with identical
	// params is a pure cache hit: no render callback runs, so collectFilledRects
	// sees only uncached chrome + blits.
	g.Draw(screen)

	rects := collectFilledRects(t, func() { g.Draw(screen) })
	t.Logf("steady-state Synth-tab filled drawRect calls: %d (budget %d)", len(rects), budget)
	if len(rects) > budget {
		t.Fatalf("steady-state Synth-tab frame issued %d filled drawRect calls (budget %d) — "+
			"a cached surface (ParamCard / chip strip / trace) regressed to per-frame rendering", len(rects), budget)
	}
}

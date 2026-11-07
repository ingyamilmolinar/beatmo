//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// A steady frame is all cache hits, so only uncached chrome and a few blits remain. Tighten the budget when more is
// cached; an increase means a surface regressed to per-frame raster and needs its reason here.
func TestSynthTabSteadyStateDrawCallBudget(t *testing.T) {
	// Measured on crop_synth_tab_modular plus headroom; reverting one cached surface trips it.
	const budget = 180

	g := driveScene(t, "crop_synth_tab_modular")
	screen := ebiten.NewImage(1280, 720)

	// Frame 1 warms every sprite; frame 2 with identical params is a pure hit.
	g.Draw(screen)

	rects := collectFilledRects(t, func() { g.Draw(screen) })
	t.Logf("steady-state Synth-tab filled drawRect calls: %d (budget %d)", len(rects), budget)
	if len(rects) > budget {
		t.Fatalf("steady-state Synth-tab frame issued %d filled drawRect calls (budget %d) — "+
			"a cached surface (ParamCard / chip strip / trace) regressed to per-frame rendering", len(rects), budget)
	}
}

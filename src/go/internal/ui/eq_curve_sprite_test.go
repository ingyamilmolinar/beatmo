//go:build test

package ui

import (
	"image/color"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

// expandEQPanelForTest gives the audio panel real height (the test-mode
// default collapses it, leaving an 8px plot in which drawEQCurve's
// minimum-size guard rejects rendering entirely).
func expandEQPanelForTest(t *testing.T, g *Game) {
	t.Helper()
	eqPanelHeightForTest = 400
	t.Cleanup(func() { eqPanelHeightForTest = 0 })
	g.Layout(1280, 720)
	// The override is consumed inside (*DrumView).layout, which runs from
	// Update — drive a few frames so the taller panel geometry lands.
	for i := 0; i < 4; i++ {
		_ = g.Update()
	}
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())
}

// curveStrokeRGBA is colEQCurve as the interceptor reports it.
func curveStrokeRGBA() color.RGBA {
	return color.RGBAModel.Convert(colEQCurve).(color.RGBA)
}

// countCurveStrokeRects counts drawRect calls carrying the curve stroke
// color, regardless of destination or coordinate space (cached renders draw
// in sprite-local coordinates; the interceptor sees them either way).
func countCurveStrokeRects(rects []drawnRect) int {
	want := curveStrokeRGBA()
	n := 0
	for _, r := range rects {
		if r.Color == want {
			n++
		}
	}
	return n
}

// TestEQCurveSpriteMatchesDirect pins pixel identity between the cached
// sprite path and the RENDER_SAFE direct path (the
// TestChainTraceCacheMatchesDirect pattern): same game state, two draws, the
// EQ plot region must be byte-identical. Exercised across a band edit so the
// key-change re-render path is covered too.
func TestEQCurveSpriteMatchesDirect(t *testing.T) {
	g := driveScene(t, "crop_eq_tab_eq")
	expandEQPanelForTest(t, g)
	z := g.drum.eqPanelZone
	if z == nil {
		t.Fatal("no EQ panel zone")
	}
	plot := z.eqPlotRect()
	if plot.Dx() < 16 || plot.Dy() < 16 {
		t.Fatalf("plot rect too small for the curve renderer: %v", plot)
	}

	compare := func(stage string) {
		t.Helper()
		cached := ebiten.NewImage(1280, 720)
		direct := ebiten.NewImage(1280, 720)
		g.Draw(cached) // sprite path (fresh dst; sprite may be warm — that's the point)

		old := envRenderSafe
		envRenderSafe = true
		g.Draw(direct)
		envRenderSafe = old

		for y := plot.Min.Y; y < plot.Max.Y; y++ {
			for x := plot.Min.X; x < plot.Max.X; x++ {
				if cached.At(x, y) != direct.At(x, y) {
					t.Fatalf("%s: pixel mismatch at (%d,%d): cached=%v direct=%v",
						stage, x, y, cached.At(x, y), direct.At(x, y))
				}
			}
		}
	}

	compare("initial")

	// Band edit → key change → re-render; identity must hold on the new curve.
	z.SetBandGainDB(2, 5.0)
	// Defeat the drag throttle deterministically.
	z.curveSprite.now = func() time.Time { return time.Now().Add(time.Second) }
	compare("after band edit")
}

// TestEQCurveSpriteRenderCount pins the caching win at the render level:
// steady-state frames must NOT re-draw the curve content (zero curve-stroke
// drawRect calls), a band edit must re-render exactly once (after the drag
// throttle window), and further steady frames go quiet again.
func TestEQCurveSpriteRenderCount(t *testing.T) {
	g := driveScene(t, "crop_eq_tab_eq")
	expandEQPanelForTest(t, g)
	z := g.drum.eqPanelZone
	screen := ebiten.NewImage(1280, 720)

	// Deterministic clock for the sprite throttle.
	now := time.Unix(10_000, 0)
	z.curveSprite.now = func() time.Time { return now }

	// Warm frame: first draw renders the curve into the sprite.
	warm := collectFilledRects(t, func() { g.Draw(screen) })
	if countCurveStrokeRects(warm) == 0 {
		t.Fatalf("warm frame drew no curve stroke — sprite render never ran (plot=%v)", z.eqPlotRect())
	}

	// Steady state: 5 frames, no edits — the curve must not re-render.
	now = now.Add(time.Second)
	steady := collectFilledRects(t, func() {
		for i := 0; i < 5; i++ {
			g.Draw(screen)
		}
	})
	if n := countCurveStrokeRects(steady); n != 0 {
		t.Fatalf("steady-state frames re-drew the curve %d times (want 0 — cached blit only)", n)
	}

	// Band edit: key changes; with the clock advanced past the throttle the
	// next frame re-renders exactly one curve pass.
	z.SetBandGainDB(1, -4.0)
	now = now.Add(time.Second)
	edited := collectFilledRects(t, func() { g.Draw(screen) })
	if countCurveStrokeRects(edited) == 0 {
		t.Fatal("band edit did not re-render the curve (stale cache)")
	}

	// And quiet again.
	next := collectFilledRects(t, func() { g.Draw(screen) })
	if n := countCurveStrokeRects(next); n != 0 {
		t.Fatalf("post-edit steady frame re-drew the curve %d times (want 0)", n)
	}
}

// TestEQTabSteadyStateDrawCallBudget pins the draw-call collapse: a
// steady-state EQ-tab frame previously issued ~2 filled drawRect calls per
// plot column for the curve alone (hundreds per frame; drawEQCurve measured
// 13.8% of process CPU, 8,441 draw calls/frame on WASM before this work).
// With the curve sprite the whole frame's filled-rect count must stay under
// the pinned budget. Measured 2026-07-14 post-sprite: see t.Log below —
// budget = measured + headroom; tighten it if you reduce calls further
// (ratchet), and justify any increase in a comment here.
func TestEQTabSteadyStateDrawCallBudget(t *testing.T) {
	// Measured 2026-07-14 with the curve+backdrop+labels sprites and the
	// production-height panel: 284 filled rects per steady frame FOR THE
	// WHOLE SCREEN (rows, transport, sliders, live bars — the EQ data
	// surfaces contribute three cached blits). The curve alone used to add
	// ~2 rects per plot column (hundreds). Ratchet: tighten when more
	// surfaces are cached; justify any increase here.
	const budget = 350

	g := driveScene(t, "crop_eq_tab_eq")
	expandEQPanelForTest(t, g)
	z := g.drum.eqPanelZone
	screen := ebiten.NewImage(1280, 720)
	now := time.Unix(20_000, 0)
	z.curveSprite.now = func() time.Time { return now }

	g.Draw(screen) // warm the sprite
	now = now.Add(time.Second)

	rects := collectFilledRects(t, func() { g.Draw(screen) })
	t.Logf("steady-state EQ-tab filled drawRect calls: %d (budget %d)", len(rects), budget)
	if len(rects) > budget {
		t.Fatalf("steady-state EQ-tab frame issued %d filled drawRect calls (budget %d) — a cached surface regressed to per-frame rendering", len(rects), budget)
	}
	// Belt-and-suspenders: none of them may be curve-stroke rects.
	if n := countCurveStrokeRects(rects); n != 0 {
		t.Fatalf("steady-state frame contains %d curve-stroke rects (want 0)", n)
	}
}

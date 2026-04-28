package ui

import (
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

/* ─────────────── Draw ─────────────────────────────────────────────────── */

func (g *Game) Draw(screen *ebiten.Image) {
	w, h := screen.Bounds().Dx(), screen.Bounds().Dy()
	if w <= 0 || h <= 0 {
		return
	}
	g.maybeYield()
	t0 := time.Now()
	gridStart := t0
	g.drawGridPane(screen) // top
	gridDur := time.Since(gridStart)
	g.maybeYield()
	drumStart := time.Now()
	g.drawDrumPane(screen) // bottom (includes buttons)
	drumDur := time.Since(drumStart)
	g.lastDrawGridMS = float64(gridDur) / 1e6
	g.lastDrawDrumMS = float64(drumDur) / 1e6
	// Draw divider last so it sits above both panes
	g.drawDivider(screen)
	if !g.perfDrawMuted {
		g.perf.onDraw(time.Since(t0))
	}
	// Screenshot mode: count draws and capture when ready.
	if g.screenshotPath != "" {
		g.screenshotDraws++
		if g.screenshotDraws == 90 {
			if err := g.captureScreen(screen); err != nil {
				g.logger.Infof("[SCREENSHOT] Error: %v", err)
			} else {
				g.logger.Infof("[SCREENSHOT] Saved to %s", g.screenshotPath)
			}
		}
	}
}

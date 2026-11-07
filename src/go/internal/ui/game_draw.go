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
	// Mobile landscape is unsupported: show the rotate notice; the screenshot still runs.
	if g.landscapeUnsupported() {
		g.drawLandscapeUnsupported(screen)
		g.maybeCaptureScreenshot(screen)
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
	// …except the window-bounded theme editor, which sits above the divider.
	g.drawThemeOverlaysAboveDivider(screen)
	if !g.perfDrawMuted {
		g.perf.onDraw(time.Since(t0))
	}
	g.maybeCaptureScreenshot(screen)
	if g.tour != nil {
		g.tour.frame(g, screen)
	}
}

// Capture and termination share screenshotThreshold, so a scene can never exit before its capture.
func (g *Game) maybeCaptureScreenshot(screen *ebiten.Image) {
	if g.screenshotPath == "" {
		return
	}
	g.screenshotDraws++
	if g.shouldCaptureScreenshot() {
		if err := g.captureScreen(screen); err != nil {
			g.logger.Infof("[screenshot] Error: %v", err)
		} else {
			g.logger.Infof("[screenshot] Saved to %s", g.screenshotPath)
		}
		// Even on error, so termination still fires.
		g.screenshotCaptured = true
	}
}

// A per-scene override wins; 0 means 90 frames.
func (g *Game) screenshotThreshold() int {
	if g.screenshotSettleFrames > 0 {
		return g.screenshotSettleFrames
	}
	return 90
}

func (g *Game) shouldCaptureScreenshot() bool {
	return g.screenshotPath != "" && !g.screenshotCaptured && g.screenshotDraws >= g.screenshotThreshold()
}

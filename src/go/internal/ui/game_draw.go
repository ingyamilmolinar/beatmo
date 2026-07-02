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
	// Mobile landscape is unsupported: render the rotate-to-portrait notice
	// instead of the (broken) portrait layout. Skips both panes entirely; the
	// screenshot capture still runs so landscape scenes can be captured.
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
	if !g.perfDrawMuted {
		g.perf.onDraw(time.Since(t0))
	}
	g.maybeCaptureScreenshot(screen)
}

// maybeCaptureScreenshot counts draws and captures the screen once the UI has
// settled. The capture frame is driven by the SAME threshold that gates
// termination (screenshotThreshold) so the two can never diverge — the pre-fix
// code captured at a hardcoded frame 90 while termination used the per-scene
// SettleFrames, so scenes with SettleFrames<90 terminated before the capture
// (no PNG) and scenes with SettleFrames>90 captured ~30+ frames too early.
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
		// Mark captured even on error so termination still fires (one attempt
		// at the settle frame; the error is logged above).
		g.screenshotCaptured = true
	}
}

// screenshotThreshold is the draw count at which the screenshot is captured and
// the game then terminates. A per-scene SetScreenshotSettleFrames override wins;
// 0 falls back to the 90-frame (~1.5s @ 60fps) default. Shared by Draw (capture
// trigger) and screenshotReady (termination) so the two stay in lockstep.
func (g *Game) screenshotThreshold() int {
	if g.screenshotSettleFrames > 0 {
		return g.screenshotSettleFrames
	}
	return 90
}

// shouldCaptureScreenshot reports whether this Draw should capture the screen:
// screenshot mode is active, the UI has settled (draw count reached the
// threshold), and no capture has fired yet.
func (g *Game) shouldCaptureScreenshot() bool {
	return g.screenshotPath != "" && !g.screenshotCaptured && g.screenshotDraws >= g.screenshotThreshold()
}

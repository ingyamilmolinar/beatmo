package ui

import (
	"image/color"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

/* ─────────────── Draw ─────────────────────────────────────────────────── */

func (g *Game) Draw(screen *ebiten.Image) {
	w, h := screen.Bounds().Dx(), screen.Bounds().Dy()
	if w <= 0 || h <= 0 {
		return
	}
	useFrameBuf := g.drawMinInterval > 0
	// Throttle Draw on web builds to reduce main-thread pressure; reuse the
	// previous frame when skipped so browsers do not flash a blank canvas.
	if g.drawMinInterval > 0 {
		now := time.Now()
		if !g.lastDrawAt.IsZero() && now.Sub(g.lastDrawAt) < g.drawMinInterval {
			if useFrameBuf && g.frameBuffer != nil {
				screen.DrawImage(g.frameBuffer, nil)
				g.drawThrottleCopies++
				g.maybeYield()
				return // only skip when we have a valid cached frame
			}
			// frameBuffer invalidated by resize — fall through to full draw
		}
		g.lastDrawAt = now
	}
	target := screen
	if useFrameBuf {
		if g.frameBuffer == nil || g.frameBufferW != w || g.frameBufferH != h {
			g.frameBuffer = ebiten.NewImage(w, h)
			g.frameBufferW, g.frameBufferH = w, h
		} else {
			g.frameBuffer.Fill(color.RGBA{})
		}
		target = g.frameBuffer
	}
	g.maybeYield()
	t0 := time.Now()
	gridStart := t0
	g.drawGridPane(target) // top
	gridDur := time.Since(gridStart)
	g.maybeYield()
	drumStart := time.Now()
	g.drawDrumPane(target) // bottom (includes buttons)
	drumDur := time.Since(drumStart)
	g.lastDrawGridMS = float64(gridDur) / 1e6
	g.lastDrawDrumMS = float64(drumDur) / 1e6
	// Draw divider last so it sits above both panes
	g.drawDivider(target)
	if !g.perfDrawMuted {
		g.perf.onDraw(time.Since(t0))
	}
	if target != screen {
		screen.DrawImage(target, nil)
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

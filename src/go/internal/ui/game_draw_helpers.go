package ui

import (
	"image"
	"image/color"
	"math"
	"runtime"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/utils"
)

// drawDivider renders a thicker horizontal divider between top and bottom panes,
// highlighting on hover to make it discoverable as draggable.
func (g *Game) drawDivider(screen *ebiten.Image) {
	mX, mY := cursorPosition()
	grab := 6
	baseCol := color.RGBA{180, 180, 180, 255}
	hovCol := color.RGBA{255, 255, 255, 255}
	thick := 2.0
	var hover bool
	if g.split.horizontal {
		hover = utils.Abs(mY-g.split.Y) <= grab
	} else {
		hover = utils.Abs(mX-g.split.X) <= grab
	}
	col := baseCol
	if hover {
		thick = 3.0
		col = hovCol
	}
	g.dividerHover = hover
	g.dividerThick = thick
	mobile := Profile().IsMobile()
	shadowCol := color.RGBA{8, 8, 10, 255}
	highCol := color.RGBA{50, 50, 58, 255}
	if g.split.horizontal {
		y := g.split.Y
		if mobile {
			drawRect(screen, image.Rect(0, y, g.winW, y+1), color.NRGBA{255, 255, 255, 20}, true)
		} else {
			drawRect(screen, image.Rect(0, y-1, g.winW, y), shadowCol, true)
			drawRect(screen, image.Rect(0, y, g.winW, y+1), col, true)
			drawRect(screen, image.Rect(0, y+1, g.winW, y+2), highCol, true)
		}
		DrawSplitterHandle(screen, g.winW/2, y, true, hover)
	} else {
		x := g.split.X
		if mobile {
			drawRect(screen, image.Rect(x, 0, x+1, g.winH), color.NRGBA{255, 255, 255, 20}, true)
		} else {
			drawRect(screen, image.Rect(x-1, 0, x, g.winH), shadowCol, true)
			drawRect(screen, image.Rect(x, 0, x+1, g.winH), col, true)
			drawRect(screen, image.Rect(x+1, 0, x+2, g.winH), highCol, true)
		}
		DrawSplitterHandle(screen, x, g.winH/2, false, hover)
	}
}

// drawCrossScreen paints a simple cross at (x,y) in screen pixels for diagnostics.
func drawCrossScreen(dst *ebiten.Image, x, y, size int, col color.Color) {
	half := size / 2
	// horizontal line
	r1 := image.Rect(x-half, y, x+half+1, y+1)
	drawRect(dst, r1, col, true)
	// vertical line
	r2 := image.Rect(x, y-half, x+1, y+half+1)
	drawRect(dst, r2, col, true)
}

func idOrNil(n *uiNode) any {
	if n == nil {
		return nil
	}
	return n.ID
}

// buildGridTile creates a stepPx×stepPx image that contains all visible grid
// subdivision lines for the current grid configuration. The tile can be
// repeated across the grid pane by translating it according to camera offset.
func (g *Game) buildGridTile(stepPx int) *ebiten.Image {
	if stepPx <= 0 {
		return nil
	}
	img := ebiten.NewImage(stepPx, stepPx)
	if g.logDrawNodes {
		g.logger.Tracef("[DRAW/GRID] buildGridTile: stepPx=%d subs=%d", stepPx, len(g.grid.Subs))
	}
	// Clear transparent (default)
	// Draw per-subdivision vertical and horizontal lines at pixel multiples.
	for _, sub := range g.grid.Subs {
		// Minimum pixel spacing: use rounded per-line positions rather than
		// integer division so rounding error is evenly distributed and aligns
		// with world-to-screen math.
		minPx := float64(stepPx) / float64(sub.Div)
		if minPx < float64(sub.MinPx) {
			continue
		}
		// Thickness in px; clamp to at least 1.
		t := sub.Style.Width
		if t < 1 {
			t = 1
		}
		thick := int(math.Round(t))
		if thick < 1 {
			thick = 1
		}
		// Vertical lines
		for k := 0; k < sub.Div; k++ {
			x := int(math.Round(float64(k) * float64(stepPx) / float64(sub.Div)))
			if x >= stepPx {
				continue
			}
			var op ebiten.DrawImageOptions
			op.GeoM.Scale(1, float64(stepPx))
			op.GeoM.Translate(float64(x), 0)
			// Expand thickness by drawing additional pixels to the right.
			for dx := 0; dx < thick; dx++ {
				op2 := op
				op2.GeoM.Translate(float64(dx), 0)
				img.DrawImage(pixel(sub.Style.Color), &op2)
			}
		}
		// Horizontal lines
		for k := 0; k < sub.Div; k++ {
			y := int(math.Round(float64(k) * float64(stepPx) / float64(sub.Div)))
			if y >= stepPx {
				continue
			}
			var op ebiten.DrawImageOptions
			op.GeoM.Scale(float64(stepPx), 1)
			op.GeoM.Translate(0, float64(y))
			for dy := 0; dy < thick; dy++ {
				op2 := op
				op2.GeoM.Translate(0, float64(dy))
				img.DrawImage(pixel(sub.Style.Color), &op2)
			}
		}
	}
	return img
}

func (g *Game) drawDrumPane(dst *ebiten.Image) {
	// Keep DrumView's seconds-per-beat in sync with the engine/app-lied BPM for
	// timeline counters without overriding the user-edited BPM control value.
	// This avoids a race where UI changes are undone by the draw loop before
	// Game.Update() can propagate them to the engine.
	bpm := g.AppliedBPM()
	if bpm <= 0 {
		bpm = g.bpm
	}
	if bpm > 0 {
		g.drum.secPerBeat = 60.0 / float64(bpm)
	}
	// Use a smooth, non-quantized beat value for timeline counters to avoid
	// jitter in the displayed timers, while highlights and steps remain
	// quantized via internal counters.
	g.drum.simpleDraw = g.simpleDraw
	g.drum.perfDrawLite = false // EQ visualizer always enabled - already optimized with early exits
	g.drum.Draw(dst, g.highlightSnapshotByRow(len(g.drum.Rows)), g.frame, g.drumBeatInfos, g.displayBeat())
}

func (g *Game) maybeYield() {
	if g == nil {
		return
	}
	if runtime.GOOS != "js" {
		return
	}
	if !g.perfMode.FastPathEnabled() {
		return
	}
	runtime.Gosched()
}
